import unittest
from publish import validate_version, release_action


class PublishTests(unittest.TestCase):
    def test_version(self):
        for value in ['0.1.0-alpha.1', '1.2.3', '2.0.0-rc.2']:
            self.assertEqual(validate_version(value), value)
        for value in ['v1.2.3', '../nightly', '1.2.3;echo secret', '1.2.3\n', 'nightly', '1.2']:
            with self.assertRaises(ValueError):
                validate_version(value)

    def test_published_versions_are_immutable(self):
        self.assertEqual(release_action(None), 'create')
        self.assertEqual(release_action({'draft': True}), 'resume')
        self.assertEqual(release_action({'draft': False}), 'skip')


class PublishFlowTests(unittest.TestCase):
    def run_flow(self, published=False, stale=False, mismatched_tag=False):
        import os
        from pathlib import Path
        from tempfile import TemporaryDirectory
        from unittest.mock import patch
        import publish
        sha = 'a' * 40
        calls = []
        commands = []
        tags = {}
        if mismatched_tag:
            tags['v0.1.0-alpha.1'] = 'b' * 40

        def fake_api(path, method=None, body=None, missing=False):
            calls.append((path, method, body))
            if path.endswith('/git/ref/heads/master'):
                return {'object': {'sha': 'b' * 40 if stale else sha}}
            if '/git/ref/tags/' in path:
                tag = path.split('/git/ref/tags/')[1]
                return {'object': {'type': 'commit', 'sha': tags[tag]}} if tag in tags else None
            if path.endswith('/git/refs'):
                tags[body['ref'].removeprefix('refs/tags/')] = body['sha']
                return {}
            if '/releases/tags/' in path:
                if published and path.endswith('/v0.1.0-alpha.1'):
                    return {'id': 1, 'draft': False, 'body': 'Commit: original-build'}
                return None
            if method == 'POST' and path.endswith('/releases'):
                return {'id': 2 if body['tag_name'] == 'nightly' else 1, 'draft': True}
            return {}

        with TemporaryDirectory() as directory:
            previous = os.getcwd()
            try:
                os.chdir(directory)
                Path('VERSION').write_text('0.1.0-alpha.1\n')
                Path('release').mkdir()
                for name in ['nodesweep-linux-amd64.tar.gz', 'nodesweep-linux-arm64.tar.gz', 'SHA256SUMS']:
                    Path('release', name).touch()
                with patch.dict(os.environ, {'GITHUB_REPOSITORY': 'While-Shark/NodeSweep',
                                             'SOURCE_SHA': sha, 'GITHUB_REF_TYPE': 'branch'}), \
                     patch.object(publish, 'render_notes', return_value='## English\n\n- Checked fixture update\n'), \
                     patch.object(publish, 'api', side_effect=fake_api), \
                     patch.object(publish.subprocess, 'run', side_effect=lambda args, **kwargs: commands.append(args)):
                    publish.main()
            finally:
                os.chdir(previous)
        return calls, commands

    def test_first_release_and_nightly_use_checked_commit(self):
        calls, commands = self.run_flow()
        refs = [body for path, method, body in calls if path.endswith('/git/refs')]
        self.assertEqual([r['ref'] for r in refs], ['refs/tags/v0.1.0-alpha.1', 'refs/tags/nightly'])
        self.assertTrue(all(r['sha'] == 'a' * 40 for r in refs))
        self.assertEqual([c[3] for c in commands if c[:3] == ['gh', 'release', 'upload']], ['v0.1.0-alpha.1', 'nightly'])
        releases = [body for path, method, body in calls if path.endswith('/releases') and method == 'POST']
        self.assertTrue(all(r['draft'] and r['prerelease'] for r in releases))

    def test_existing_version_is_not_uploaded_again(self):
        calls, commands = self.run_flow(published=True)
        self.assertEqual([c[3] for c in commands if c[:3] == ['gh', 'release', 'upload']], ['nightly'])
        note_updates = [body for path, method, body in calls if path.endswith('/releases/1') and method == 'PATCH']
        self.assertEqual(len(note_updates), 1)
        self.assertTrue(note_updates[0]['body'].startswith('Commit: original-build'))
        self.assertIn('## English', note_updates[0]['body'])

    def test_stale_source_does_not_publish(self):
        calls, commands = self.run_flow(stale=True)
        self.assertEqual(commands, [])
        self.assertTrue(all(method is None for _, method, _ in calls))

    def test_tag_mismatch_fails_before_writes(self):
        with self.assertRaisesRegex(ValueError, 'different commit'):
            self.run_flow(mismatched_tag=True)

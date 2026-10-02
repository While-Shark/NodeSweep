import json
from pathlib import Path
import tempfile
import unittest
from release_notes import LANGUAGES, render_notes


class ReleaseNotesTests(unittest.TestCase):
    def test_current_notes_match_version_and_cover_all_languages(self):
        version = Path('VERSION').read_text().strip()
        notes = render_notes(version)
        for _, label in LANGUAGES:
            self.assertIn('## ' + label + '\n\n- ', notes)

    def test_wrong_version_is_rejected(self):
        with self.assertRaisesRegex(ValueError, 'match VERSION'):
            render_notes('99.0.0')

    def test_missing_language_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            file = Path(directory, 'notes.json')
            file.write_text(json.dumps({'version':'1.0.0','changes':{'en':['Example']}}))
            with self.assertRaisesRegex(ValueError, 'five languages'):
                render_notes('1.0.0', file)

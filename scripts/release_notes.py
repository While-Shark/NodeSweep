"""Render the reviewed five-language notes into GitHub Release Markdown."""
import json
from pathlib import Path

LANGUAGES = [('en', 'English'), ('zh-CN', '简体中文'), ('zh-TW', '繁體中文'),
             ('ja', '日本語'), ('ko', '한국어')]


def render_notes(version, path=Path('docs/release-notes.json')):
    data = json.loads(Path(path).read_text())
    if data.get('version') != version:
        raise ValueError('Release notes must match VERSION')
    notes = data.get('changes')
    if not isinstance(notes, dict) or set(notes) != {code for code, _ in LANGUAGES}:
        raise ValueError('Release notes require all five languages')
    sections = []
    for code, label in LANGUAGES:
        items = notes[code]
        if not isinstance(items, list) or not items or any(
            not isinstance(item, str) or not item.strip() or '\n' in item for item in items
        ):
            raise ValueError('Each language requires nonempty single-line change descriptions')
        sections.append('## ' + label + '\n\n' + '\n'.join('- ' + item for item in items))
    return '\n\n'.join(sections) + '\n'


if __name__ == '__main__':
    print(render_notes(Path('VERSION').read_text().strip()))

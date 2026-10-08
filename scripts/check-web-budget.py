#!/usr/bin/env python3
"""Check entry/login resource budgets from Vite's production manifest.

Count each static JS/CSS resource once and gzip each file independently at level
9. Unvisited dynamic routes are excluded; this is a loading budget, not a timing
or actual wire-compression measurement. Uses only the Python standard library.
"""
import argparse
import gzip
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parent.parent
JS_LIMIT = 160 * 1024
CSS_LIMIT = 25 * 1024


def initial_files(manifest, roots):
    seen, files = set(), set()
    pending = list(roots)
    while pending:
        key = pending.pop()
        if key in seen:
            continue
        if key not in manifest:
            raise ValueError('Build manifest is missing a required chunk: ' + key)
        seen.add(key)
        chunk = manifest[key]
        files.add(chunk['file'])
        files.update(chunk.get('css', []))
        pending.extend(chunk.get('imports', []))
    return files


def measure(dist, files):
    sizes = {'js': 0, 'css': 0}
    for name in sorted(files):
        suffix = Path(name).suffix.lstrip('.')
        if suffix in sizes:
            sizes[suffix] += len(gzip.compress((dist / name).read_bytes(), compresslevel=9, mtime=0))
    return sizes


def within_budget(sizes, js_limit, css_limit):
    return sizes['js'] <= js_limit and sizes['css'] <= css_limit


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dist', type=Path, default=ROOT / 'web/dist')
    arguments = parser.parse_args()
    try:
        manifest = json.loads((arguments.dist / '.vite/manifest.json').read_text())
        passed = True
        for label, roots in [('entry', ['index.html']), ('login', ['index.html', 'src/views/AuthView.vue'])]:
            files = initial_files(manifest, roots)
            sizes = measure(arguments.dist, files)
            accepted = within_budget(sizes, JS_LIMIT, CSS_LIMIT)
            passed = passed and accepted
            print(f"{label}: JS gzip {sizes['js']} / {JS_LIMIT} bytes; CSS gzip {sizes['css']} / {CSS_LIMIT} bytes; {'PASS' if accepted else 'OVER BUDGET'}")
        return 0 if passed else 1
    except (OSError, ValueError, KeyError, TypeError) as error:
        print('Web budget check failed; build with a valid Vite manifest first: ' + str(error), file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())

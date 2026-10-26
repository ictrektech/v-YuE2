#!/usr/bin/env python3
"""Preserve dependency license texts alongside a source/lockfile inventory."""
import argparse
import json
from pathlib import Path
import shutil

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--source', type=Path, required=True)
p.add_argument('--vendor', type=Path, required=True)
p.add_argument('--output', type=Path, required=True)
args = p.parse_args()
records = []
for kind, source in [('rust', args.vendor), ('npm', args.source / 'app/node_modules')]:
    for item in source.rglob('*'):
        if not item.is_file() or not item.name.upper().startswith(('LICENSE', 'LICENCE', 'COPYING', 'NOTICE')):
            continue
        target = args.output / kind / item.relative_to(source)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(item, target)
        records.append(str(target.relative_to(args.output)))
args.output.mkdir(parents=True, exist_ok=True)
for item in ['Cargo.lock', 'app/package-lock.json']:
    shutil.copyfile(args.source / item, args.output / Path(item).name)
(args.output / 'index.json').write_text(json.dumps({'source': 'https://github.com/ictrektech/v-YuE2', 'notices': sorted(records)}, indent=2) + '\n')

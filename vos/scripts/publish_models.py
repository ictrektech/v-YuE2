#!/usr/bin/env python3
"""Mirror pinned, verified model files on a Linux build host; never download on macOS."""
import argparse
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import shutil
import urllib.request

ROOT = Path(__file__).resolve().parents[2]

def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda: f.read(8 << 20), b''):
            h.update(chunk)
    return h.hexdigest()

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--build-root', type=Path, required=True)
    p.add_argument('--credentials', type=Path, required=True)
    p.add_argument('--publish', action='store_true')
    args = p.parse_args()
    assert os.uname().sysname == 'Linux', 'Use a Linux build host'
    assert args.build_root.resolve().is_relative_to((Path.home() / 'dev/build').resolve())
    assert not args.credentials.stat().st_mode & 0o077
    keys = json.loads(args.credentials.read_text())
    catalog = json.loads((ROOT / 'vos/models.json').read_text())
    out = args.build_root / 'publication'
    out.mkdir(parents=True, exist_ok=True)
    from modelscope_hub import HubApi
    def stage(spec):
        target = out / spec['path']
        def valid():
            return target.is_file() and target.stat().st_size == spec['size'] and digest(target) == spec['sha256']
        if valid():
            return
        print('Download ' + spec['path'], flush=True)
        for domain in ('huggingface.co', 'hf-mirror.com'):
            part = target.with_suffix(target.suffix + '.part')
            for attempt in range(12):
                offset = part.stat().st_size if part.exists() else 0
                if offset == spec['size']:
                    if digest(part) == spec['sha256']:
                        part.replace(target)
                        print('Verified ' + spec['path'], flush=True)
                        return
                    part.unlink()
                    offset = 0
                if offset > spec['size']:
                    part.unlink()
                    offset = 0
                url = f"https://{domain}/{spec['repo']}/resolve/{spec['revision']}/{spec['path']}?download=true&resume={offset}"
                headers = {'User-Agent': 'v-YuE2 model publisher'}
                if keys.get('hf'):
                    headers['Authorization'] = 'Bearer ' + keys['hf']
                if offset:
                    headers['Range'] = f'bytes={offset}-'
                try:
                    with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=30) as response:
                        if response.status == 206:
                            content_range = response.headers.get('Content-Range', '')
                            if not content_range.startswith(f'bytes {offset}-') or not content_range.endswith('/' + str(spec['size'])):
                                raise ValueError('invalid resume range')
                        elif response.status == 200:
                            offset = 0
                        else:
                            raise ValueError('unexpected download status')
                        with part.open('ab' if offset else 'wb') as f:
                            shutil.copyfileobj(response, f, 8 << 20)
                    if part.stat().st_size == spec['size'] and digest(part) == spec['sha256']:
                        part.replace(target)
                        print('Verified ' + spec['path'], flush=True)
                        return
                    print('Resume ' + spec['path'] + ' at ' + str(part.stat().st_size), flush=True)
                except Exception as error:
                    print('Source unavailable: ' + domain + ' ' + spec['path'] + ' ' + type(error).__name__ + ' ' + str(getattr(error, 'code', '')), flush=True)
                    if not part.exists() or part.stat().st_size == offset:
                        break
        raise RuntimeError('No verified source: ' + spec['path'])
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        list(pool.map(stage, catalog['files']))
    shutil.copytree(ROOT / 'vos/licenses', out / 'licenses', dirs_exist_ok=True)
    shutil.copyfile(ROOT / 'vos/models.json', out / 'source-index.json')
    shutil.copyfile(ROOT / 'vos/MODEL_README.md', out / 'README.md')
    (out / 'SHA256SUMS').write_text(''.join(f"{f['sha256']}  {f['path']}\n" for f in catalog['files']))
    if not args.publish:
        return
    api = HubApi(token=keys['modelscope'])
    repo = catalog['modelscope_repo']
    try:
        api.get_repo(repo, 'model')
    except Exception:
        api.create_repo(repo, 'model', visibility='public', license='other', description='YuE2 GGUF mirror; original non-commercial model licenses apply')
    api.upload_folder(repo, 'model', out, sync_remote_repo=False, disable_tqdm=True, max_workers=2, commit_message='Mirror verified YuE2 weights with original licenses and source hashes')
    remote = {f.path: f for f in HubApi().list_repo_files(repo, 'model')}
    for f in catalog['files']:
        actual = remote.get(f['path'])
        assert actual and actual.sha256 == f['sha256'] and actual.size == f['size'], f['path']
    print(f"Anonymous publication audit passed: {repo}, {len(catalog['files'])} files", flush=True)

if __name__ == '__main__':
    main()

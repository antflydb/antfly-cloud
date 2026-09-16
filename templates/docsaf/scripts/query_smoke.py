#!/usr/bin/env python3
from __future__ import annotations
import json, os, pathlib, urllib.request, urllib.error
ROOT = pathlib.Path(__file__).resolve().parents[1]
for name in ('.env.local', '.env'):
    p = ROOT / name
    if p.exists():
        for raw in p.read_text().splitlines():
            if raw.strip() and not raw.strip().startswith('#') and '=' in raw:
                k, v = raw.split('=', 1); os.environ.setdefault(k.strip(), v.strip().strip('"').strip("'"))
base = os.environ.get('ANTFLY_URL', '').rstrip('/')
key = os.environ.get('ANTFLYDB_API_KEY', '')
table = os.environ.get('DOCSAF_TABLE', 'docsaf')
if not base or not key:
    raise SystemExit('ANTFLY_URL and ANTFLYDB_API_KEY are required')
body = {'semantic_search': os.environ.get('QUERY', 'source-document rows hierarchy artifacts'), 'limit': 5}
req = urllib.request.Request(base + f'/tables/{table}/query', data=json.dumps(body).encode(), headers={'Authorization': 'Bearer ' + key, 'Content-Type': 'application/json', 'Accept': 'application/json'}, method='POST')
try:
    with urllib.request.urlopen(req, timeout=60) as resp:
        print(resp.read().decode())
except urllib.error.HTTPError as e:
    print(e.read().decode('utf-8', 'replace'))
    raise

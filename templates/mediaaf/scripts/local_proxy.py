#!/usr/bin/env python3
"""Tiny local Antfly proxy that keeps ANTFLYDB_API_KEY out of browser bundles."""
from __future__ import annotations
import http.server, os, pathlib, urllib.request, urllib.error

ROOT = pathlib.Path(__file__).resolve().parents[1]

def load_env(path: pathlib.Path) -> None:
    if not path.exists():
        return
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith('#') or '=' not in line:
            continue
        k, v = line.split('=', 1)
        os.environ.setdefault(k.strip(), v.strip().strip('"').strip("'"))

load_env(ROOT / '.env.local')
load_env(ROOT / '.env')
ANTFLY_URL = os.environ.get('ANTFLY_URL', '').rstrip('/')
ANTFLYDB_API_KEY = os.environ.get('ANTFLYDB_API_KEY', '')

class Handler(http.server.BaseHTTPRequestHandler):
    def do_OPTIONS(self):
        self.send_response(204); self._cors(); self.end_headers()
    def do_GET(self): self._proxy()
    def do_POST(self): self._proxy()
    def do_DELETE(self): self._proxy()
    def _cors(self):
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET,POST,DELETE,OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Content-Type,Authorization')
    def _proxy(self):
        if not ANTFLY_URL or not ANTFLYDB_API_KEY:
            self.send_error(500, 'ANTFLY_URL and ANTFLYDB_API_KEY must be set in .env.local')
            return
        length = int(self.headers.get('content-length') or 0)
        body = self.rfile.read(length) if length else None
        path = self.path
        if ANTFLY_URL.endswith('/api/v1') and path.startswith('/api/v1'):
            path = path[len('/api/v1'):] or '/'
        target = ANTFLY_URL + path
        headers = {'Authorization': 'Bearer ' + ANTFLYDB_API_KEY, 'Accept': 'application/json'}
        if body is not None:
            headers['Content-Type'] = self.headers.get('content-type', 'application/json')
        req = urllib.request.Request(target, data=body, headers=headers, method=self.command)
        try:
            with urllib.request.urlopen(req, timeout=120) as resp:
                raw = resp.read()
                self.send_response(resp.status)
                self._cors(); self.send_header('Content-Type', resp.headers.get('content-type', 'application/json')); self.end_headers(); self.wfile.write(raw)
        except urllib.error.HTTPError as e:
            raw = e.read()
            self.send_response(e.code); self._cors(); self.send_header('Content-Type', 'application/json'); self.end_headers(); self.wfile.write(raw)

if __name__ == '__main__':
    port = int(os.environ.get('PORT', '8765'))
    print(f'MediaAF proxy on http://127.0.0.1:{port} -> {ANTFLY_URL}')
    http.server.ThreadingHTTPServer(('127.0.0.1', port), Handler).serve_forever()

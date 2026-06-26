#!/usr/bin/env python3
from __future__ import annotations
import http.server, os, pathlib, urllib.request, urllib.error
ROOT = pathlib.Path(__file__).resolve().parents[1]
for name in ('.env.local', '.env'):
    p = ROOT / name
    if p.exists():
        for raw in p.read_text().splitlines():
            if raw.strip() and not raw.strip().startswith('#') and '=' in raw:
                k, v = raw.split('=', 1); os.environ.setdefault(k.strip(), v.strip().strip('"').strip("'"))
BASE = os.environ.get('ANTFLY_URL', '').rstrip('/')
KEY = os.environ.get('ANTFLYDB_API_KEY', '')
class H(http.server.BaseHTTPRequestHandler):
    def do_OPTIONS(self): self.send_response(204); self.cors(); self.end_headers()
    def do_GET(self): self.proxy()
    def do_POST(self): self.proxy()
    def cors(self):
        self.send_header('Access-Control-Allow-Origin','*'); self.send_header('Access-Control-Allow-Methods','GET,POST,OPTIONS'); self.send_header('Access-Control-Allow-Headers','Content-Type')
    def proxy(self):
        if not BASE or not KEY: self.send_error(500, 'ANTFLY_URL and ANTFLYDB_API_KEY required'); return
        body = self.rfile.read(int(self.headers.get('content-length') or 0)) or None
        req = urllib.request.Request(BASE + self.path, data=body, headers={'Authorization':'Bearer '+KEY, 'Content-Type': self.headers.get('content-type','application/json'), 'Accept':'application/json'}, method=self.command)
        try:
            with urllib.request.urlopen(req, timeout=120) as r:
                data = r.read(); self.send_response(r.status); self.cors(); self.send_header('Content-Type', r.headers.get('content-type','application/json')); self.end_headers(); self.wfile.write(data)
        except urllib.error.HTTPError as e:
            data = e.read(); self.send_response(e.code); self.cors(); self.send_header('Content-Type','application/json'); self.end_headers(); self.wfile.write(data)
if __name__ == '__main__':
    port = int(os.environ.get('PORT','8766'))
    print(f'DocsAF proxy on http://127.0.0.1:{port} -> {BASE}')
    http.server.ThreadingHTTPServer(('127.0.0.1', port), H).serve_forever()

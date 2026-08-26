# 2026-08-26 — Phase 4 (first slice): port preview

GET /api/projects/{p}/ports scans the guest's /proc/net/tcp{,6} via exec
(LISTEN, ports ≥1024); /proxy/{p}/{port}/ reverse-proxies to the microVM's
bridge IP (cached ReverseProxy per target, WS pass-through, behind
authGuard). Right rail shows clickable :port chips. Gotcha: a method-less
ServeMux pattern conflicts with the "GET /" catch-all — proxy registered
per-method. Verified live: python http.server on :3000 inside the demo
microVM detected and served through the proxy; integration 23/23, e2e 18/18.

/* Brewhouse service worker — cache shell/static; API always hits network. */
const SW_VERSION = new URL(self.location.href).searchParams.get("v") || "dev";
const CACHE_NAME = "brewhouse-" + SW_VERSION;

const PRECACHE = [
	"/",
	"/static/css/app.css",
	"/static/js/app.js",
	"/static/js/wasm_loader.js",
	"/static/js/wasm_exec.js",
	"/static/wasm/app.wasm",
	"/static/manifest.webmanifest",
	"/static/images/icon-192.png",
	"/static/images/icon-512.png",
	"/logo",
	"/favicon",
];

self.addEventListener("install", (event) => {
	event.waitUntil(
		caches
			.open(CACHE_NAME)
			.then((cache) => cache.addAll(PRECACHE))
			.then(() => self.skipWaiting())
	);
});

self.addEventListener("activate", (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) =>
				Promise.all(
					keys
						.filter((key) => key.startsWith("brewhouse-") && key !== CACHE_NAME)
						.map((key) => caches.delete(key))
				)
			)
			.then(() => self.clients.claim())
	);
});

self.addEventListener("fetch", (event) => {
	const req = event.request;
	if (req.method !== "GET") {
		return;
	}
	const url = new URL(req.url);
	if (url.origin !== self.location.origin) {
		return;
	}
	if (url.pathname.startsWith("/api/")) {
		return;
	}

	event.respondWith(
		caches.match(req).then((cached) => {
			if (cached) {
				return cached;
			}
			return fetch(req)
				.then((res) => {
					if (res && res.ok && (url.pathname.startsWith("/static/") || url.pathname === "/" || url.pathname === "/logo" || url.pathname === "/favicon")) {
						const copy = res.clone();
						caches.open(CACHE_NAME).then((cache) => cache.put(req, copy));
					}
					return res;
				})
				.catch(() => {
					if (req.mode === "navigate") {
						return caches.match("/");
					}
					return undefined;
				});
		})
	);
});

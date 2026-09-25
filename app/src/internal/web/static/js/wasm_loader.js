(() => {
	const app = document.getElementById("app");
	const progressFill = document.querySelector(".splash__progress-fill");
	const splashError = document.getElementById("splash-error");
	const loginForm = document.getElementById("login-form");
	const loginError = document.getElementById("login-error");
	const loginSuccess = document.getElementById("login-success");
	const loginStage = document.querySelector(".login-stage");
	const submitButton = loginForm ? loginForm.querySelector('button[type="submit"]') : null;
	const bootstrapBox = document.getElementById("bootstrap-credentials");
	const bootstrapUsername = document.getElementById("bootstrap-username");
	const bootstrapPassword = document.getElementById("bootstrap-password");
	const shell = document.getElementById("shell");
	const shellCollapse = document.getElementById("shell-collapse");
	const shellAvatar = document.getElementById("shell-avatar");
	const shellProfileName = document.getElementById("shell-profile-name");
	const shellProfile = document.getElementById("shell-profile");
	const profileWrap = document.getElementById("shell-profile-wrap");
	const profileMenu = document.getElementById("shell-profile-menu");
	const profileViewBtn = document.getElementById("shell-profile-view");
	const profileLogoutBtn = document.getElementById("shell-profile-logout");
	const profileAdminWrap = document.getElementById("shell-profile-admin-wrap");
	const profileAdminToggle = document.getElementById("shell-profile-admin");
	const profileDialog = document.getElementById("profile-dialog");
	const profileForm = document.getElementById("profile-form");
	const profileError = document.getElementById("profile-error");
	const brandHome = document.getElementById("shell-brand-home");
	const topbarBrand = document.getElementById("shell-topbar-brand");
	const navFab = document.getElementById("shell-nav-fab");
	const navBackdrop = document.getElementById("shell-nav-backdrop");
	const navSheet = document.getElementById("shell-nav-sheet");
	const fabIcon = navFab ? navFab.querySelector(".shell__nav-fab-icon") : null;

	let wasmReady = false;
	let barDone = false;
	let wasmFailed = false;

	const IDLE_MS = 10 * 60 * 1000;
	const SESSION_CHECK_MS = 60 * 1000;
	const REFRESH_AGE_MS = 5 * 60 * 1000;
	const EXPIRING_SOON_MS = 90 * 1000;

	let lastActivity = Date.now();
	let sessionTimer = null;
	let refreshInFlight = null;

	function authHeaders() {
		const token = sessionStorage.getItem("brewhouse_token") || "";
		return {
			"Content-Type": "application/json",
			Authorization: "Bearer " + token,
		};
	}

	function markActivity() {
		lastActivity = Date.now();
	}

	function parseTokenClaims() {
		try {
			const t = sessionStorage.getItem("brewhouse_token") || "";
			const part = t.split(".")[1];
			if (!part) {
				return null;
			}
			const padded = part + "=".repeat((4 - (part.length % 4)) % 4);
			const json = atob(padded.replace(/-/g, "+").replace(/_/g, "/"));
			return JSON.parse(json);
		} catch {
			return null;
		}
	}

	function applySessionData(data) {
		if (!data || !data.token) {
			return;
		}
		sessionStorage.setItem("brewhouse_token", data.token);
		if (data.user_id != null) {
			sessionStorage.setItem("brewhouse_user_id", String(data.user_id));
		}
		if (data.role) {
			sessionStorage.setItem("brewhouse_role", data.role);
		}
		sessionStorage.setItem("brewhouse_can_elevate", data.can_elevate ? "1" : "0");
		if (data.username) {
			sessionStorage.setItem("brewhouse_username", data.username);
		}
	}

	async function refreshSession() {
		if (refreshInFlight) {
			return refreshInFlight;
		}
		const token = sessionStorage.getItem("brewhouse_token");
		if (!token) {
			return false;
		}
		refreshInFlight = (async () => {
			try {
				const res = await fetch("/api/session/refresh", {
					method: "POST",
					headers: authHeaders(),
				});
				const data = await res.json().catch(() => ({}));
				if (!res.ok) {
					return false;
				}
				applySessionData(data);
				applyNavVisibility(data.role);
				syncAdminToggle();
				return true;
			} catch {
				return false;
			} finally {
				refreshInFlight = null;
			}
		})();
		return refreshInFlight;
	}

	function tickSession() {
		const token = sessionStorage.getItem("brewhouse_token");
		if (!token || !app.classList.contains("is-shell")) {
			return;
		}
		const now = Date.now();
		if (now - lastActivity > IDLE_MS) {
			logout();
			return;
		}
		const claims = parseTokenClaims();
		if (!claims || !claims.exp) {
			logout();
			return;
		}
		const untilExp = claims.exp * 1000 - now;
		if (untilExp <= 0) {
			logout();
			return;
		}
		const age = claims.iat ? now - claims.iat * 1000 : REFRESH_AGE_MS;
		if (age >= REFRESH_AGE_MS || untilExp <= EXPIRING_SOON_MS) {
			refreshSession().then((ok) => {
				if (!ok) {
					logout();
				}
			});
		}
	}

	function startSessionWatch() {
		markActivity();
		if (sessionTimer) {
			return;
		}
		sessionTimer = setInterval(tickSession, SESSION_CHECK_MS);
	}

	function stopSessionWatch() {
		if (sessionTimer) {
			clearInterval(sessionTimer);
			sessionTimer = null;
		}
		refreshInFlight = null;
	}

	function currentRole() {
		return (sessionStorage.getItem("brewhouse_role") || "").trim().toLowerCase();
	}

	function canElevate() {
		return sessionStorage.getItem("brewhouse_can_elevate") === "1";
	}

	function syncAdminToggle() {
		if (!profileAdminWrap || !profileAdminToggle) {
			return;
		}
		const elevatable = canElevate();
		profileAdminWrap.hidden = !elevatable;
		profileAdminWrap.classList.toggle("is-role-hidden", !elevatable);
		if (elevatable) {
			profileAdminToggle.checked = currentRole() === "admin";
		}
		syncAdminTips();
	}

	function syncAdminTips() {
		const tip = document.getElementById("shell-welcome-admin-tip");
		if (tip) {
			tip.hidden = !canElevate();
		}
	}

	function applyNavVisibility(role) {
		const r = (role || currentRole() || "").toLowerCase();
		document.querySelectorAll("[data-nav-roles]").forEach((el) => {
			const allowed = (el.getAttribute("data-nav-roles") || "")
				.split(",")
				.map((s) => s.trim().toLowerCase())
				.filter(Boolean);
			const show = r && allowed.includes(r);
			el.classList.toggle("is-role-hidden", !show);
			if (!show) {
				el.classList.remove("is-expanded");
				const header = el.querySelector("[aria-expanded]");
				if (header) {
					header.setAttribute("aria-expanded", "false");
				}
			}
		});
	}

	window.BrewhouseAuth = {
		role: currentRole,
		token: () => sessionStorage.getItem("brewhouse_token") || "",
		canElevate,
		can: (roles) => {
			const r = currentRole();
			if (!r) {
				return false;
			}
			const list = Array.isArray(roles) ? roles : String(roles).split(",");
			return list.map((s) => s.trim().toLowerCase()).includes(r);
		},
		applyNavVisibility,
		applyWelcomeLogoBg,
		markActivity,
		refreshSession,
		logout: () => logout(),
	};

	function hideBootstrapCredentials() {
		if (bootstrapBox) {
			bootstrapBox.hidden = true;
		}
		if (bootstrapUsername) {
			bootstrapUsername.value = "";
		}
		if (bootstrapPassword) {
			bootstrapPassword.value = "";
		}
	}

	async function loadBootstrapCredentials() {
		if (!bootstrapBox) {
			return;
		}
		try {
			const response = await fetch("/api/bootstrap");
			const data = await response.json().catch(() => ({}));
			if (!response.ok || !data.pending) {
				hideBootstrapCredentials();
				return;
			}
			if (bootstrapUsername) {
				bootstrapUsername.value = data.username || "admin";
			}
			if (bootstrapPassword) {
				bootstrapPassword.value = data.password || "";
			}
			bootstrapBox.hidden = false;
			if (loginForm && loginForm.username && data.username) {
				loginForm.username.value = data.username;
			}
		} catch (err) {
			console.error("Bootstrap credentials request failed:", err);
			hideBootstrapCredentials();
		}
	}

	function maybeShowLogin() {
		if (wasmFailed || app.classList.contains("is-shell")) {
			return;
		}
		if (wasmReady && barDone) {
			const existing = sessionStorage.getItem("brewhouse_token");
			const username = sessionStorage.getItem("brewhouse_username");
			if (existing && username) {
				const claims = parseTokenClaims();
				if (!claims || !claims.exp || claims.exp * 1000 <= Date.now()) {
					sessionStorage.removeItem("brewhouse_token");
					sessionStorage.removeItem("brewhouse_username");
					sessionStorage.removeItem("brewhouse_user_id");
					sessionStorage.removeItem("brewhouse_role");
					sessionStorage.removeItem("brewhouse_can_elevate");
				} else {
					showShell(username);
					return;
				}
			}
			app.classList.add("is-login");
			loadBootstrapCredentials();
		}
	}

	function showLoadError(message) {
		wasmFailed = true;
		splashError.hidden = false;
		splashError.textContent = message;
	}

	function showLoginError(message) {
		loginError.hidden = false;
		loginError.textContent = message;
		loginSuccess.hidden = true;
	}

	function setSheetOpen(open) {
		shell.classList.toggle("is-sheet-open", open);
		document.body.classList.toggle("is-nav-sheet-open", open);
		navFab.setAttribute("aria-expanded", open ? "true" : "false");
		navFab.setAttribute("aria-label", open ? "Close navigation" : "Open navigation");
		navSheet.setAttribute("aria-hidden", open ? "false" : "true");
		navBackdrop.hidden = !open;
		if (fabIcon) {
			fabIcon.textContent = open ? "close" : "menu";
		}
	}

	function setProfileMenuOpen(open) {
		if (!profileMenu || !shellProfile || !profileWrap) {
			return;
		}
		profileMenu.hidden = !open;
		profileWrap.classList.toggle("is-open", open);
		shellProfile.setAttribute("aria-expanded", open ? "true" : "false");
		if (open) {
			syncAdminToggle();
		}
	}

	function applyWelcomeLogoBg(hex) {
		const color = hex || "#6c704a";
		if (shell) {
			shell.style.setProperty("--welcome-logo-bg", color);
		}
		document.documentElement.style.setProperty("--welcome-logo-bg", color);
	}

	async function loadWelcomeLogoBg() {
		try {
			const res = await fetch("/api/settings/brand-color", { headers: authHeaders() });
			const data = await res.json().catch(() => ({}));
			if (!res.ok) {
				return;
			}
			applyWelcomeLogoBg(data.logo_bg_hex);
		} catch (err) {
			console.error(err);
		}
	}

	function showShell(username) {
		app.classList.remove("is-login");
		app.classList.add("is-shell");
		setSheetOpen(false);
		setProfileMenuOpen(false);
		applyNavVisibility(currentRole());
		syncAdminToggle();
		loadWelcomeLogoBg();
		startSessionWatch();

		const initial = (username || "?").charAt(0).toUpperCase();
		shellAvatar.textContent = initial;
		shellProfileName.textContent = username;
		shellProfile.setAttribute("aria-label", "Profile: " + username);
	}

	function logout() {
		stopSessionWatch();
		sessionStorage.removeItem("brewhouse_token");
		sessionStorage.removeItem("brewhouse_username");
		sessionStorage.removeItem("brewhouse_user_id");
		sessionStorage.removeItem("brewhouse_role");
		sessionStorage.removeItem("brewhouse_can_elevate");
		setProfileMenuOpen(false);
		setSheetOpen(false);
		app.classList.remove("is-shell");
		app.classList.add("is-login");
		loginStage.classList.remove("is-authenticated");
		loginSuccess.hidden = true;
		loginError.hidden = true;
		if (loginForm) {
			loginForm.reset();
		}
		hideBootstrapCredentials();
		loadBootstrapCredentials();
	}

	function welcomeHTML() {
		const tip = canElevate()
			? '<p class="shell__welcome-tip" id="shell-welcome-admin-tip">Admin accounts: open your profile menu and turn on <strong>Admin mode</strong> for Economy, IAM, and Settings.</p>'
			: "";
		return (
			'<section class="shell__welcome">' +
			'<div class="shell__welcome-logo-wrap">' +
			'<span class="shell__welcome-logo-bg" aria-hidden="true"></span>' +
			'<img class="shell__welcome-logo" src="/logo" alt="" width="120" height="120">' +
			"</div>" +
			'<p class="shell__welcome-brand">Brewhouse</p>' +
			'<h1 class="shell__welcome-title">From recipe to the pub</h1>' +
			'<p class="shell__welcome-text">Pick a section in the navigation, or start with the batch pipeline guide.</p>' +
			'<a class="btn btn--primary shell__welcome-cta" href="/app/guide" hx-get="/app/guide" hx-target="#main-content" hx-swap="innerHTML">Brewery 101</a>' +
			tip +
			"</section>"
		);
	}

	function showHome() {
		const main = document.getElementById("main-content");
		if (!main) {
			return;
		}
		main.innerHTML = welcomeHTML();
		if (window.htmx) {
			window.htmx.process(main);
		}
		history.replaceState(null, "", "/");
		setProfileMenuOpen(false);
		setSheetOpen(false);
	}

	function setSidebarCollapsed(collapsed) {
		if (!shell) {
			return;
		}
		shell.classList.toggle("is-collapsed", !!collapsed);
		if (shellCollapse) {
			shellCollapse.setAttribute("aria-expanded", collapsed ? "false" : "true");
			shellCollapse.setAttribute(
				"aria-label",
				collapsed ? "Expand navigation" : "Collapse navigation"
			);
		}
		if (collapsed) {
			shell.querySelectorAll(".shell__nav-section.is-expanded").forEach((section) => {
				section.classList.remove("is-expanded");
				const header = section.querySelector(".shell__nav-header");
				if (header) {
					header.setAttribute("aria-expanded", "false");
				}
			});
		}
	}

	function leaveAdminOnlyPageIfNeeded() {
		const main = document.getElementById("main-content");
		const panel = main ? main.querySelector("[data-panel]") : null;
		const kind = panel ? panel.getAttribute("data-panel") : "";
		if (kind === "iam" || (kind && kind.startsWith("iam-")) || (kind && kind.startsWith("settings"))) {
			showHome();
		}
	}

	async function setElevated(elevated) {
		const res = await fetch("/api/session/elevate", {
			method: "POST",
			headers: authHeaders(),
			body: JSON.stringify({ elevated: !!elevated }),
		});
		const data = await res.json().catch(() => ({}));
		if (!res.ok) {
			throw new Error(data.error || "Could not change admin mode");
		}
		applySessionData(data);
		applyNavVisibility(data.role);
		syncAdminToggle();
		markActivity();
		if (!elevated) {
			leaveAdminOnlyPageIfNeeded();
		}
	}

	async function openProfileDialog() {
		setProfileMenuOpen(false);
		profileError.hidden = true;
		profileForm.password.value = "";
		profileForm.password_confirm.value = "";
		try {
			const res = await fetch("/api/me", { headers: authHeaders() });
			const data = await res.json().catch(() => ({}));
			if (!res.ok) {
				if (window.BrewhouseUI && typeof window.BrewhouseUI.info === "function") {
					await window.BrewhouseUI.info({
						title: "Notice",
						message: data.error || "Could not load profile",
					});
				}
				return;
			}
			profileForm.username.value = data.username || "";
			profileForm.email.value = data.email || "";
			profileForm.first_name.value = data.first_name || "";
			profileForm.last_name.value = data.last_name || "";
			profileForm.address_line1.value = data.address_line1 || "";
			profileForm.address_line2.value = data.address_line2 || "";
			profileForm.phone.value = data.phone || "";
			profileForm.instagram.value = data.instagram || "";
			profileDialog.showModal();
		} catch (err) {
			console.error(err);
			if (window.BrewhouseUI && typeof window.BrewhouseUI.info === "function") {
				await window.BrewhouseUI.info({
					title: "Notice",
					message: "Could not load profile",
				});
			}
		}
	}

	function showLoginSuccess(username) {
		loginError.hidden = true;
		loginSuccess.hidden = false;
		loginSuccess.textContent = "Signed in as " + username;
		loginStage.classList.add("is-authenticated");
		sessionStorage.setItem("brewhouse_username", username);
		setTimeout(() => {
			showShell(username);
		}, 2000);
	}

	if (progressFill) {
		progressFill.addEventListener("animationend", () => {
			barDone = true;
			maybeShowLogin();
		});
	}

	if (shellCollapse) {
		shellCollapse.addEventListener("click", () => {
			setSidebarCollapsed(!shell.classList.contains("is-collapsed"));
		});
	}

	[brandHome, topbarBrand].forEach((btn) => {
		if (!btn) {
			return;
		}
		btn.addEventListener("click", () => {
			showHome();
		});
	});

	if (navFab) {
		navFab.addEventListener("click", () => {
			setSheetOpen(!shell.classList.contains("is-sheet-open"));
		});
	}

	if (navBackdrop) {
		navBackdrop.addEventListener("click", () => {
			setSheetOpen(false);
		});
	}

	if (shellProfile) {
		shellProfile.addEventListener("click", (event) => {
			event.stopPropagation();
			setProfileMenuOpen(profileMenu.hidden);
		});
	}

	if (profileViewBtn) {
		profileViewBtn.addEventListener("click", () => {
			openProfileDialog();
		});
	}

	if (profileLogoutBtn) {
		profileLogoutBtn.addEventListener("click", () => {
			logout();
		});
	}

	if (profileAdminToggle) {
		profileAdminToggle.addEventListener("change", async () => {
			const want = profileAdminToggle.checked;
			try {
				await setElevated(want);
				setProfileMenuOpen(false);
			} catch (err) {
				profileAdminToggle.checked = !want;
				if (window.BrewhouseUI && typeof window.BrewhouseUI.info === "function") {
					await window.BrewhouseUI.info({
						title: "Notice",
						message: err.message || "Could not change admin mode",
					});
				}
			}
		});
	}

	document.addEventListener("click", (event) => {
		if (!profileWrap || profileMenu.hidden) {
			return;
		}
		if (!profileWrap.contains(event.target)) {
			setProfileMenuOpen(false);
		}
	});

	document.addEventListener("keydown", (event) => {
		markActivity();
		if (event.key === "Escape") {
			if (shell.classList.contains("is-sheet-open")) {
				setSheetOpen(false);
			}
			setProfileMenuOpen(false);
		}
	});

	["pointerdown", "mousemove", "touchstart", "scroll"].forEach((evt) => {
		document.addEventListener(evt, markActivity, { passive: true });
	});
	document.addEventListener("visibilitychange", () => {
		if (document.visibilityState === "visible") {
			markActivity();
		}
	});

	if (profileDialog && profileForm) {
		profileDialog.addEventListener("close", async () => {
			if (profileDialog.returnValue !== "save") {
				return;
			}
			profileError.hidden = true;
			const email = profileForm.email.value.trim();
			const password = profileForm.password.value;
			const passwordConfirm = profileForm.password_confirm.value;
			if (password !== passwordConfirm) {
				profileError.hidden = false;
				profileError.textContent = "Passwords do not match";
				profileDialog.showModal();
				return;
			}
			const body = {
				email,
				first_name: profileForm.first_name.value.trim(),
				last_name: profileForm.last_name.value.trim(),
				address_line1: profileForm.address_line1.value.trim(),
				address_line2: profileForm.address_line2.value.trim(),
				phone: profileForm.phone.value.trim(),
				instagram: profileForm.instagram.value.trim(),
			};
			if (password) {
				body.password = password;
			}
			try {
				const res = await fetch("/api/me", {
					method: "PATCH",
					headers: authHeaders(),
					body: JSON.stringify(body),
				});
				const data = await res.json().catch(() => ({}));
				if (!res.ok) {
					profileError.hidden = false;
					profileError.textContent = data.error || "Save failed";
					profileDialog.showModal();
					return;
				}
				profileForm.password.value = "";
				profileForm.password_confirm.value = "";
			} catch (err) {
				profileError.hidden = false;
				profileError.textContent = "Could not reach the server";
				profileDialog.showModal();
			}
		});
	}

	function expandExclusive(section, sectionSelector, header) {
		const root = section.parentElement;
		if (!root) {
			return;
		}
		root.querySelectorAll(sectionSelector).forEach((other) => {
			if (other === section) {
				return;
			}
			other.classList.remove("is-expanded");
			const otherHeader = other.querySelector("[aria-expanded]");
			if (otherHeader) {
				otherHeader.setAttribute("aria-expanded", "false");
			}
		});
		const open = section.classList.toggle("is-expanded");
		header.setAttribute("aria-expanded", open ? "true" : "false");
	}

	function openNavSection(section, sectionSelector, header) {
		const root = section.parentElement;
		if (!root) {
			return;
		}
		root.querySelectorAll(sectionSelector).forEach((other) => {
			if (other === section) {
				return;
			}
			other.classList.remove("is-expanded");
			const otherHeader = other.querySelector("[aria-expanded]");
			if (otherHeader) {
				otherHeader.setAttribute("aria-expanded", "false");
			}
		});
		section.classList.add("is-expanded");
		header.setAttribute("aria-expanded", "true");
	}

	function bindExclusiveAccordion(root, headerSelector, sectionSelector, options) {
		if (!root) {
			return;
		}
		const expandSidebarWhenCollapsed = !!(options && options.expandSidebarWhenCollapsed);
		root.querySelectorAll(headerSelector).forEach((header) => {
			header.addEventListener("click", () => {
				const section = header.closest(sectionSelector);
				if (!section) {
					return;
				}
				if (
					expandSidebarWhenCollapsed &&
					shell &&
					shell.classList.contains("is-collapsed")
				) {
					setSidebarCollapsed(false);
					openNavSection(section, sectionSelector, header);
					return;
				}
				expandExclusive(section, sectionSelector, header);
			});
		});
	}

	bindExclusiveAccordion(
		document.querySelector(".shell__nav"),
		".shell__nav-header",
		".shell__nav-section",
		{ expandSidebarWhenCollapsed: true }
	);
	bindExclusiveAccordion(navSheet, ".shell__sheet-header", ".shell__sheet-section");

	document.body.addEventListener("htmx:afterOnLoad", () => {
		if (shell && shell.classList.contains("is-sheet-open")) {
			setSheetOpen(false);
		}
	});

	if (loginForm) {
		loginForm.addEventListener("submit", async (event) => {
			event.preventDefault();
			loginError.hidden = true;

			const username = loginForm.username.value.trim();
			const password = loginForm.password.value;

			if (submitButton) {
				submitButton.disabled = true;
			}

			try {
				const response = await fetch("/api/login", {
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ username, password }),
				});

				const data = await response.json().catch(() => ({}));
				if (!response.ok) {
					showLoginError(data.error || "Login failed");
					return;
				}

				applySessionData(data);
				hideBootstrapCredentials();
				showLoginSuccess(data.username);
			} catch (err) {
				console.error("Login request failed:", err);
				showLoginError("Could not reach the server");
			} finally {
				if (submitButton) {
					submitButton.disabled = false;
				}
			}
		});
	}

	const go = new Go();

	WebAssembly.instantiateStreaming(fetch("/static/wasm/app.wasm"), go.importObject)
		.then((result) => {
			go.run(result.instance);
			wasmReady = true;
			maybeShowLogin();
		})
		.catch((err) => {
			console.error("Failed to load WASM:", err);
			showLoadError("Failed to load application. Refresh and try again.");
		});
})();

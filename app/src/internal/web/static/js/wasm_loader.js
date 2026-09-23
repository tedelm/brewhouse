(() => {
	const app = document.getElementById("app");
	const progressFill = document.querySelector(".splash__progress-fill");
	const splashError = document.getElementById("splash-error");
	const loginForm = document.getElementById("login-form");
	const loginError = document.getElementById("login-error");
	const loginSuccess = document.getElementById("login-success");
	const loginStage = document.querySelector(".login-stage");
	const submitButton = loginForm ? loginForm.querySelector('button[type="submit"]') : null;
	const shell = document.getElementById("shell");
	const shellCollapse = document.getElementById("shell-collapse");
	const shellAvatar = document.getElementById("shell-avatar");
	const shellProfileName = document.getElementById("shell-profile-name");
	const shellProfile = document.getElementById("shell-profile");
	const profileWrap = document.getElementById("shell-profile-wrap");
	const profileMenu = document.getElementById("shell-profile-menu");
	const profileViewBtn = document.getElementById("shell-profile-view");
	const profileLogoutBtn = document.getElementById("shell-profile-logout");
	const profileDialog = document.getElementById("profile-dialog");
	const profileForm = document.getElementById("profile-form");
	const profileError = document.getElementById("profile-error");
	const navFab = document.getElementById("shell-nav-fab");
	const navBackdrop = document.getElementById("shell-nav-backdrop");
	const navSheet = document.getElementById("shell-nav-sheet");
	const fabIcon = navFab ? navFab.querySelector(".shell__nav-fab-icon") : null;

	let wasmReady = false;
	let barDone = false;
	let wasmFailed = false;

	function authHeaders() {
		const token = sessionStorage.getItem("brewhouse_token") || "";
		return {
			"Content-Type": "application/json",
			Authorization: "Bearer " + token,
		};
	}

	function currentRole() {
		return (sessionStorage.getItem("brewhouse_role") || "").trim().toLowerCase();
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
		can: (roles) => {
			const r = currentRole();
			if (!r) {
				return false;
			}
			const list = Array.isArray(roles) ? roles : String(roles).split(",");
			return list.map((s) => s.trim().toLowerCase()).includes(r);
		},
		applyNavVisibility,
	};

	function maybeShowLogin() {
		if (wasmFailed || app.classList.contains("is-shell")) {
			return;
		}
		if (wasmReady && barDone) {
			const existing = sessionStorage.getItem("brewhouse_token");
			const username = sessionStorage.getItem("brewhouse_username");
			if (existing && username) {
				showShell(username);
				return;
			}
			app.classList.add("is-login");
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
	}

	function showShell(username) {
		app.classList.remove("is-login");
		app.classList.add("is-shell");
		setSheetOpen(false);
		setProfileMenuOpen(false);
		applyNavVisibility(currentRole());

		const initial = (username || "?").charAt(0).toUpperCase();
		shellAvatar.textContent = initial;
		shellProfileName.textContent = username;
		shellProfile.setAttribute("aria-label", "Profile: " + username);
	}

	function logout() {
		sessionStorage.removeItem("brewhouse_token");
		sessionStorage.removeItem("brewhouse_username");
		sessionStorage.removeItem("brewhouse_user_id");
		sessionStorage.removeItem("brewhouse_role");
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
	}

	async function openProfileDialog() {
		setProfileMenuOpen(false);
		profileError.hidden = true;
		profileForm.password.value = "";
		try {
			const res = await fetch("/api/me", { headers: authHeaders() });
			const data = await res.json().catch(() => ({}));
			if (!res.ok) {
				alert(data.error || "Could not load profile");
				return;
			}
			profileForm.username.value = data.username || "";
			profileForm.email.value = data.email || "";
			profileDialog.showModal();
		} catch (err) {
			console.error(err);
			alert("Could not load profile");
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
			const collapsed = shell.classList.toggle("is-collapsed");
			shellCollapse.setAttribute("aria-expanded", collapsed ? "false" : "true");
			shellCollapse.setAttribute(
				"aria-label",
				collapsed ? "Expand navigation" : "Collapse navigation"
			);
			if (collapsed) {
				shell.querySelectorAll(".shell__nav-section.is-expanded").forEach((section) => {
					section.classList.remove("is-expanded");
					const header = section.querySelector(".shell__nav-header");
					if (header) {
						header.setAttribute("aria-expanded", "false");
					}
				});
			}
		});
	}

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

	document.addEventListener("click", (event) => {
		if (!profileWrap || profileMenu.hidden) {
			return;
		}
		if (!profileWrap.contains(event.target)) {
			setProfileMenuOpen(false);
		}
	});

	document.addEventListener("keydown", (event) => {
		if (event.key === "Escape") {
			if (shell.classList.contains("is-sheet-open")) {
				setSheetOpen(false);
			}
			setProfileMenuOpen(false);
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
			const body = { email };
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

	function bindExclusiveAccordion(root, headerSelector, sectionSelector) {
		if (!root) {
			return;
		}
		root.querySelectorAll(headerSelector).forEach((header) => {
			header.addEventListener("click", () => {
				const section = header.closest(sectionSelector);
				if (!section) {
					return;
				}
				expandExclusive(section, sectionSelector, header);
			});
		});
	}

	bindExclusiveAccordion(
		document.querySelector(".shell__nav"),
		".shell__nav-header",
		".shell__nav-section"
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

				sessionStorage.setItem("brewhouse_token", data.token);
				if (data.user_id != null) {
					sessionStorage.setItem("brewhouse_user_id", String(data.user_id));
				}
				if (data.role) {
					sessionStorage.setItem("brewhouse_role", data.role);
				}
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

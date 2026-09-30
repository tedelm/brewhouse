(() => {
	const TOKEN_KEY = "brewhouse_token";

	function token() {
		return sessionStorage.getItem(TOKEN_KEY) || "";
	}

	async function api(path, options = {}) {
		if (window.BrewhouseAuth && typeof window.BrewhouseAuth.markActivity === "function") {
			window.BrewhouseAuth.markActivity();
		}

		const doFetch = async () => {
			const headers = Object.assign(
				{ "Content-Type": "application/json" },
				options.headers || {},
				{ Authorization: "Bearer " + token() }
			);
			const res = await fetch(path, Object.assign({}, options, { headers }));
			const text = await res.text();
			let data = null;
			try {
				data = text ? JSON.parse(text) : null;
			} catch {
				data = { error: text };
			}
			return { res, data };
		};

		let { res, data } = await doFetch();
		if (res.status === 401 && path !== "/api/session/refresh") {
			const auth = window.BrewhouseAuth;
			if (auth && typeof auth.refreshSession === "function") {
				const ok = await auth.refreshSession();
				if (ok) {
					({ res, data } = await doFetch());
				} else if (typeof auth.logout === "function") {
					auth.logout();
				}
			}
		}
		if (!res.ok) {
			const err = new Error((data && data.error) || res.statusText);
			err.status = res.status;
			err.data = data;
			throw err;
		}
		return data;
	}

	async function downloadCSVAuth(path, filename) {
		const res = await fetch(path, {
			headers: { Authorization: "Bearer " + token() },
		});
		if (!res.ok) {
			const text = await res.text();
			let msg = res.statusText;
			try {
				const data = text ? JSON.parse(text) : null;
				if (data && data.error) {
					msg = data.error;
				}
			} catch {
				/* ignore */
			}
			throw new Error(msg);
		}
		const blob = await res.blob();
		const a = document.createElement("a");
		a.href = URL.createObjectURL(blob);
		a.download = filename;
		a.click();
		URL.revokeObjectURL(a.href);
	}

	async function importCSVAuth(path, file) {
		const res = await fetch(path, {
			method: "POST",
			headers: {
				Authorization: "Bearer " + token(),
				"Content-Type": "text/csv",
			},
			body: file,
		});
		const text = await res.text();
		let data = null;
		try {
			data = text ? JSON.parse(text) : null;
		} catch {
			data = { error: text };
		}
		if (!res.ok) {
			throw new Error((data && data.error) || res.statusText);
		}
		return data;
	}

	function formatImportResult(result) {
		const parts = [
			t("js.import.created", { n: result.created || 0 }),
			t("js.import.updated", { n: result.updated || 0 }),
			t("js.import.failed", { n: result.failed || 0 }),
		];
		if (result.errors && result.errors.length) {
			parts.push("", t("js.import.errors"), result.errors.join("\n"));
		}
		return parts.join("\n");
	}

	function esc(s) {
		return String(s ?? "")
			.replace(/&/g, "&amp;")
			.replace(/</g, "&lt;")
			.replace(/>/g, "&gt;")
			.replace(/"/g, "&quot;");
	}

	function statusPill(status, label) {
		const s = String(status || "").trim();
		const cls = s.toLowerCase().replace(/\s+/g, "_");
		return (
			'<span class="status-pill status-pill--' +
			esc(cls) +
			'">' +
			esc(label != null ? label : s) +
			"</span>"
		);
	}

	function table(headers, rowsHtml) {
		const body = Array.isArray(rowsHtml) ? rowsHtml.join("") : rowsHtml;
		return (
			'<div class="table-scroll"><table class="data-table"><thead><tr>' +
			headers.map((h) => "<th>" + esc(h) + "</th>").join("") +
			"</tr></thead><tbody>" +
			body +
			"</tbody></table></div>"
		);
	}

	function fmtMoney(n) {
		if (n == null || n === "" || Number.isNaN(Number(n))) {
			return "";
		}
		const amount = Number(n).toFixed(2);
		const suffix =
			window.BH_I18N && typeof window.BH_I18N.currencySuffix === "function"
				? window.BH_I18N.currencySuffix()
				: " SEK";
		return amount + suffix;
	}

	function fmtMoneyAmount(n) {
		if (n == null || n === "" || Number.isNaN(Number(n))) {
			return "";
		}
		return Number(n).toFixed(2);
	}

	function t(key, vars) {
		if (window.BH_I18N && typeof window.BH_I18N.t === "function") {
			return window.BH_I18N.t(key, vars);
		}
		return key;
	}

	function isPasswordComplex(password) {
		if (!password || [...password].length < 8) {
			return false;
		}
		return /[^A-Za-z0-9]/.test(password);
	}

	function currencyCode() {
		if (window.BH_I18N && typeof window.BH_I18N.currencyCode === "function") {
			return window.BH_I18N.currencyCode();
		}
		return "SEK";
	}

	function currencyPerLiter() {
		if (window.BH_I18N && typeof window.BH_I18N.currencyPerLiter === "function") {
			return window.BH_I18N.currencyPerLiter();
		}
		return "SEK/L";
	}

	function noticeTitle() {
		return t("js.notice");
	}

	function statusText(status) {
		const s = String(status || "").trim();
		if (!s) {
			return "";
		}
		const key = "js.status." + s.replace(/\s+/g, "_");
		const translated = t(key);
		return translated === key ? s : translated;
	}

	function categoryText(category) {
		const c = String(category || "")
			.trim()
			.toLowerCase();
		if (!c) {
			return "";
		}
		const key = "js.category." + c;
		const translated = t(key);
		return translated === key ? category : translated;
	}

	async function syncRegionalFromServer() {
		if (!token() || !window.BH_I18N) {
			return;
		}
		try {
			const cfg = await api("/api/settings/regional");
			await window.BH_I18N.applyRegional(cfg);
		} catch (_) {
			/* ignore until authenticated / seeded */
		}
	}

	function fmtSG(v, fallback) {
		if (v == null || v === "" || Number.isNaN(Number(v))) {
			return fallback != null ? fallback : "";
		}
		return Number(v).toFixed(3);
	}

	function datePart(iso) {
		if (!iso) {
			return "";
		}
		const s = String(iso);
		const t = s.indexOf("T");
		return t >= 0 ? s.slice(0, t) : s.slice(0, 10);
	}

	function recipeABV(r) {
		if (r.og == null || r.fg == null) {
			return "";
		}
		return ((r.og - r.fg) * 131.25).toFixed(1) + "%";
	}

	function appConfirm(opts) {
		const dialog = document.getElementById("app-confirm-dialog");
		const titleEl = document.getElementById("app-confirm-title");
		const messageEl = document.getElementById("app-confirm-message");
		if (!dialog || !titleEl || !messageEl) {
			return Promise.resolve(false);
		}
		titleEl.textContent = (opts && opts.title) || t("js.confirm");
		messageEl.textContent = (opts && opts.message) || "";
		return new Promise((resolve) => {
			const onClose = () => {
				dialog.removeEventListener("close", onClose);
				resolve(dialog.returnValue === "confirm");
			};
			dialog.addEventListener("close", onClose);
			dialog.showModal();
		});
	}

	function appInfo(opts) {
		const dialog = document.getElementById("app-info-dialog");
		const titleEl = document.getElementById("app-info-title");
		const messageEl = document.getElementById("app-info-message");
		if (!dialog || !titleEl || !messageEl) {
			return Promise.resolve();
		}
		titleEl.textContent = (opts && opts.title) || t("js.info");
		messageEl.textContent = (opts && opts.message) || "";
		return new Promise((resolve) => {
			const onClose = () => {
				dialog.removeEventListener("close", onClose);
				resolve();
			};
			dialog.addEventListener("close", onClose);
			dialog.showModal();
		});
	}

	function pipelineGuide(step) {
		const map = {
			recipe_created: {
				title: t("js.pipeline.recipe_created.title"),
				message: t("js.pipeline.recipe_created.message"),
			},
			scheduled: {
				title: t("js.pipeline.scheduled.title"),
				message: t("js.pipeline.scheduled.message"),
			},
			brewday_recorded: {
				title: t("js.pipeline.brewday.title"),
				message: t("js.pipeline.brewday.message"),
			},
			hygiene_done: {
				title: t("js.pipeline.hygiene.title"),
				message: t("js.pipeline.hygiene.message"),
			},
			ready_for_delivery: {
				title: t("js.pipeline.ready.title"),
				message: t("js.pipeline.ready.message"),
			},
			delivered: {
				title: t("js.pipeline.delivered.title"),
				message: t("js.pipeline.delivered.message"),
			},
		};
		return map[step] || { title: t("js.next_step"), message: "" };
	}

	function appPrompt(opts) {
		const dialog = document.getElementById("app-prompt-dialog");
		const form = document.getElementById("app-prompt-form");
		const titleEl = document.getElementById("app-prompt-title");
		const fieldsEl = document.getElementById("app-prompt-fields");
		if (!dialog || !form || !titleEl || !fieldsEl) {
			return Promise.resolve(null);
		}
		const fields = (opts && opts.fields) || [];
		titleEl.textContent = (opts && opts.title) || t("js.input");
		fieldsEl.innerHTML = fields
			.map((f) => {
				const type = f.type || "text";
				const required = f.required === false ? "" : " required";
				const label =
					"<label>" + esc(f.label || f.name) + " ";
				if (type === "select") {
					const options = (f.options || [])
						.map((o) => {
							const selected =
								f.value != null && String(f.value) === String(o.value)
									? " selected"
									: "";
							return (
								'<option value="' +
								esc(String(o.value)) +
								'"' +
								selected +
								">" +
								esc(String(o.label != null ? o.label : o.value)) +
								"</option>"
							);
						})
						.join("");
					return (
						label +
						'<select name="' +
						esc(f.name) +
						'"' +
						required +
						">" +
						options +
						"</select></label>"
					);
				}
				const step = f.step != null ? ' step="' + esc(String(f.step)) + '"' : "";
				const min = f.min != null ? ' min="' + esc(String(f.min)) + '"' : "";
				const value = f.value != null ? esc(String(f.value)) : "";
				return (
					label +
					'<input name="' +
					esc(f.name) +
					'" type="' +
					esc(type) +
					'"' +
					step +
					min +
					required +
					' value="' +
					value +
					'"></label>'
				);
			})
			.join("");
		return new Promise((resolve) => {
			const onClose = () => {
				dialog.removeEventListener("close", onClose);
				if (dialog.returnValue !== "save") {
					resolve(null);
					return;
				}
				const fd = new FormData(form);
				const out = {};
				fields.forEach((f) => {
					out[f.name] = String(fd.get(f.name) ?? "");
				});
				resolve(out);
			};
			dialog.addEventListener("close", onClose);
			dialog.showModal();
		});
	}

	window.BrewhouseUI = {
		info: appInfo,
		confirm: appConfirm,
		prompt: appPrompt,
		refreshOrdersNavCount,
		refreshBrewingNavCounts,
	};

	async function refreshOrdersNavCount() {
		const els = document.querySelectorAll("[data-orders-nav-count]");
		if (!els.length) {
			return;
		}
		try {
			const data = await api("/api/inventory/orders/open-count");
			const n = data && typeof data.count === "number" ? data.count : 0;
			els.forEach((el) => {
				if (n > 0) {
					el.textContent = String(n);
					el.hidden = false;
				} else {
					el.textContent = "";
					el.hidden = true;
				}
			});
		} catch {
			els.forEach((el) => {
				el.textContent = "";
				el.hidden = true;
			});
		}
	}

	function setNavCount(selector, n) {
		document.querySelectorAll(selector).forEach((el) => {
			if (n > 0) {
				el.textContent = String(n);
				el.hidden = false;
			} else {
				el.textContent = "";
				el.hidden = true;
			}
		});
	}

	async function refreshBrewingNavCounts() {
		const hasAny =
			document.querySelector("[data-recipes-nav-count]") ||
			document.querySelector("[data-schedule-nav-count]") ||
			document.querySelector("[data-brewday-nav-count]") ||
			document.querySelector("[data-delivery-nav-count]");
		if (!hasAny) {
			return;
		}
		try {
			const data = await api("/api/recipes/nav-counts");
			setNavCount(
				"[data-recipes-nav-count]",
				data && typeof data.recipes === "number" ? data.recipes : 0
			);
			setNavCount(
				"[data-schedule-nav-count]",
				data && typeof data.schedule === "number" ? data.schedule : 0
			);
			setNavCount(
				"[data-brewday-nav-count]",
				data && typeof data.brewday === "number" ? data.brewday : 0
			);
			setNavCount(
				"[data-delivery-nav-count]",
				data && typeof data.delivery === "number" ? data.delivery : 0
			);
		} catch {
			setNavCount("[data-recipes-nav-count]", 0);
			setNavCount("[data-schedule-nav-count]", 0);
			setNavCount("[data-brewday-nav-count]", 0);
			setNavCount("[data-delivery-nav-count]", 0);
		}
	}

	function canRoles(roles) {
		if (window.BrewhouseAuth && typeof window.BrewhouseAuth.can === "function") {
			return window.BrewhouseAuth.can(roles);
		}
		const r = (sessionStorage.getItem("brewhouse_role") || "").trim().toLowerCase();
		if (!r) {
			return false;
		}
		const list = Array.isArray(roles) ? roles : String(roles).split(",");
		return list.map((s) => s.trim().toLowerCase()).includes(r);
	}

	function applyRequireRoles(root) {
		root.querySelectorAll("[data-require-roles]").forEach((el) => {
			const allowed = el.getAttribute("data-require-roles") || "";
			el.classList.toggle("is-role-hidden", !canRoles(allowed));
		});
		const elevatable =
			window.BrewhouseAuth && typeof window.BrewhouseAuth.canElevate === "function"
				? window.BrewhouseAuth.canElevate()
				: sessionStorage.getItem("brewhouse_can_elevate") === "1";
		root.querySelectorAll("[data-require-elevate]").forEach((el) => {
			el.classList.toggle("is-role-hidden", !elevatable);
		});
	}

	function showForbidden(panel) {
		panel.innerHTML =
			'<p class="panel__empty">' + esc(t("js.forbidden")) + "</p>";
	}

	document.body.addEventListener("htmx:configRequest", (event) => {
		if (window.BrewhouseAuth && typeof window.BrewhouseAuth.markActivity === "function") {
			window.BrewhouseAuth.markActivity();
		}
		const t = token();
		if (t) {
			event.detail.headers.Authorization = "Bearer " + t;
		}
	});

	document.body.addEventListener("htmx:responseError", async (event) => {
		const xhr = event.detail.xhr;
		if (!xhr || xhr.status !== 401) {
			return;
		}
		const auth = window.BrewhouseAuth;
		if (!auth || typeof auth.refreshSession !== "function") {
			return;
		}
		const ok = await auth.refreshSession();
		if (!ok && typeof auth.logout === "function") {
			auth.logout();
		}
	});

	document.body.addEventListener("htmx:afterSwap", (event) => {
		if (event.detail.target && event.detail.target.id === "main-content") {
			if (window.BH_I18N && typeof window.BH_I18N.applyI18n === "function") {
				window.BH_I18N.applyI18n(event.detail.target);
			}
			initPanel(event.detail.target);
			if (window.Brewhouse && typeof window.Brewhouse.onNavigate === "function") {
				window.Brewhouse.onNavigate();
			}
		}
	});

	function initPanel(root) {
		const panel = root.querySelector("[data-panel]");
		if (!panel) {
			return;
		}
		applyRequireRoles(panel);
		const kind = panel.getAttribute("data-panel");
		if (kind === "iam-users" && !canRoles("admin")) {
			showForbidden(panel);
			return;
		}
		if ((kind === "iam" || (kind && kind.startsWith("iam-"))) && kind !== "iam-breweries" && !canRoles("admin")) {
			showForbidden(panel);
			return;
		}
		if ((kind && kind.startsWith("settings")) && !canRoles("admin")) {
			showForbidden(panel);
			return;
		}
		if ((kind === "economy" || kind === "economy-deliveries") && !canRoles("admin")) {
			showForbidden(panel);
			return;
		}
		const loaders = {
			recipes: loadRecipes,
			schedule: loadSchedule,
			brewday: loadBrewday,
			inventory: loadInventory,
			orders: loadOrders,
			hygiene: loadHygiene,
			"tools-calculators": loadToolsCalculators,
			economy: loadEconomy,
			"economy-deliveries": loadEconomyDeliveries,
			delivery: loadDelivery,
			"iam-users": loadIAMUsers,
			"iam-breweries": loadIAMBreweries,
			"settings-brand": loadSettingsBrand,
			"settings-tanks": loadSettingsTanks,
			"settings-multipliers": loadSettingsMultipliers,
			"settings-suppliers": loadSettingsSuppliers,
			"settings-beer-price": loadSettingsBeerPrice,
			"settings-regional": loadSettingsRegional,
			"settings-hygiene": loadSettingsHygiene,
			"settings-backup": loadSettingsBackup,
		};
		const fn = loaders[kind];
		if (fn) {
			fn(panel);
		}
	}

	async function loadRecipes(panel) {
		const list = panel.querySelector("#recipes-list");
		const dialog = panel.querySelector("#recipe-dialog");
		const form = panel.querySelector("#recipe-form");
		const errEl = panel.querySelector("#recipe-error");
		const titleEl = panel.querySelector("#recipe-dialog-title");
		const saveBtn = panel.querySelector("#recipe-save-btn");
		const brewerySel = form.querySelector('[name="brewery_id"]');
		const shortfallDialog = panel.querySelector("#recipe-shortfall-dialog");
		const shortfallList = panel.querySelector("#recipe-shortfall-list");
		const shortfallNotice = panel.querySelector("#recipe-shortfall-notice");
		const shortfallError = panel.querySelector("#recipe-shortfall-error");
		const showHiddenEl = panel.querySelector("#recipes-show-hidden");

		function editableStatus(status) {
			return status === "created" || status === "scheduled";
		}

		function showHidden() {
			return !!(showHiddenEl && showHiddenEl.checked);
		}

		async function openRecipeDialog(recipe) {
			errEl.hidden = true;
			const breweries = await api("/api/breweries");
			const items = await api("/api/inventory");
			form._items = items || [];
			brewerySel.innerHTML = (breweries || [])
				.map((b) => '<option value="' + b.id + '">' + esc(b.name) + "</option>")
				.join("");
			renderIngSections(panel);
			if (recipe && recipe.id) {
				form.elements.namedItem("id").value = String(recipe.id);
				brewerySel.value = String(recipe.brewery_id);
				brewerySel.disabled = true;
				form.elements.namedItem("name").value = recipe.name || "";
				if (titleEl) {
					titleEl.textContent = t("js.recipes.edit");
				}
				if (saveBtn) {
					saveBtn.textContent = t("common.save");
				}
				const ings = recipe.ingredients || [];
				ings.forEach((ing) =>
					addIngRow(panel, ing.inventory_item_id, ing.qty, ing.unit, ing.category)
				);
			} else {
				form.elements.namedItem("id").value = "";
				brewerySel.disabled = false;
				form.elements.namedItem("name").value = "";
				if (titleEl) {
					titleEl.textContent = t("js.recipes.new");
				}
				if (saveBtn) {
					saveBtn.textContent = t("js.recipes.create");
				}
			}
			dialog.showModal();
		}

		async function refresh() {
			list.textContent = t("js.loading");
			try {
				const qs = showHidden() ? "?include_hidden=1" : "";
				const recipes = await api("/api/recipes" + qs);
				if (!recipes || !recipes.length) {
					list.innerHTML = "<p class=\"panel__empty\">" + esc(t("js.recipes.empty")) + "</p>";
					refreshBrewingNavCounts();
					return;
				}
				function ingredientsUsed(status) {
					return (
						status === "brewday" ||
						status === "hygiene_done" ||
						status === "ready_for_delivery" ||
						status === "delivered"
					);
				}
				function lineStatus(ing, recipeStatus) {
					if (ingredientsUsed(recipeStatus)) {
						return "completed";
					}
					const need = Number(ing.qty) || 0;
					const taken = Number(ing.checked_out) || 0;
					if (taken >= need) {
						return "ok";
					}
					if (taken > 0) {
						return "partial";
					}
					return "short";
				}
				function statusChip(status) {
					const s = status || "ok";
					return (
						'<span class="status-pill status-pill--' +
						esc(s) +
						" ingredient-status ingredient-status--" +
						esc(s) +
						'">' +
						esc(s) +
						"</span>"
					);
				}
				function renderIngDetail(r) {
					const ings = r.ingredients || [];
					if (!ings.length) {
						return '<p class="panel__empty">' + esc(t("js.recipes.no_ingredients")) + "</p>";
					}
					const rows = ings
						.map((ing) => {
							const unit = ing.unit ? " " + esc(ing.unit) : "";
							return (
								"<tr><td>" +
								esc(ing.item_name || "") +
								"</td><td>" +
								esc(categoryText(ing.category || "")) +
								"</td><td>" +
								(ing.qty ?? "") +
								unit +
								"</td><td>" +
								(ing.checked_out ?? "") +
								unit +
								"</td><td>" +
								statusChip(lineStatus(ing, r.status)) +
								"</td></tr>"
							);
						})
						.join("");
					return (
						'<table class="data-table data-table--nested"><thead><tr>' +
						"<th>" + esc(t("js.recipes.ing.ingredient")) + "</th><th>" + esc(t("js.recipes.ing.category")) + "</th><th>" + esc(t("js.recipes.ing.need")) + "</th><th>" + esc(t("js.recipes.ing.checked_out")) + "</th><th>" + esc(t("js.recipes.ing.status")) + "</th>" +
						"</tr></thead><tbody>" +
						rows +
						"</tbody></table>"
					);
				}
				const body = recipes
					.map((r) => {
						let actions = "";
						if (editableStatus(r.status)) {
							actions +=
								'<button type="button" class="btn btn--small" data-edit-recipe="' +
								r.id +
								'">' + esc(t("common.edit")) + "</button> ";
							actions +=
								'<button type="button" class="btn btn--small" data-del-recipe="' +
								r.id +
								'">' + esc(t("common.delete")) + "</button>";
						}
						if (r.status === "delivered" && r.active !== false) {
							actions +=
								' <button type="button" class="btn btn--small" data-hide-recipe="' +
								r.id +
								'">' + esc(t("js.recipes.hide")) + "</button>";
						}
						if (r.status === "delivered" && r.active === false) {
							actions +=
								' <button type="button" class="btn btn--small" data-unhide-recipe="' +
								r.id +
								'">' + esc(t("js.recipes.unhide")) + "</button>";
						}
						if (r.status === "ready_for_delivery") {
							actions +=
								' <button type="button" class="btn btn--small" data-deliver="' +
								r.id +
								'">' + esc(t("js.recipes.deliver")) + "</button>";
						}
						const statusLabel =
							r.status === "delivered" && r.active === false
								? t("js.recipes.delivered_hidden")
								: statusText(r.status);
						const statusKey =
							r.status === "delivered" && r.active === false ? "delivered" : r.status;
						const ingStatus = r.ingredient_status || "ok";
						const main =
							'<tr class="recipe-row" data-recipe-id="' +
							r.id +
							'"><td>' +
							r.id +
							"</td><td>" +
							esc(r.name) +
							"</td><td>" +
							esc(r.brewery_name || String(r.brewery_id)) +
							"</td><td>" +
							statusPill(statusKey, statusLabel) +
							'</td><td><button type="button" class="ingredient-status-btn" data-toggle-ingredients="' +
							r.id +
							'" aria-expanded="false">' +
							statusChip(ingStatus) +
							' <span class="ingredient-status-btn__chevron" aria-hidden="true">▸</span></button></td><td>' +
							actions +
							"</td></tr>";
						const detail =
							'<tr class="recipe-ings-detail" data-ings-for="' +
							r.id +
							'" hidden><td colspan="6">' +
							renderIngDetail(r) +
							"</td></tr>";
						return main + detail;
					})
					.join("");
				list.innerHTML =
					'<div class="table-scroll"><table class="data-table"><thead><tr>' +
					"<th>" + esc(t("js.recipes.col.id")) + "</th><th>" + esc(t("js.recipes.col.name")) + "</th><th>" + esc(t("js.recipes.col.brewery")) + "</th><th>" + esc(t("js.recipes.col.status")) + "</th><th>" + esc(t("js.recipes.col.ingredients")) + "</th><th>" + esc(t("js.recipes.col.actions")) + "</th>" +
					"</tr></thead><tbody>" +
					body +
					"</tbody></table></div>";
				refreshBrewingNavCounts();
			} catch (e) {
				list.textContent = e.message;
			}
		}

		function showShortfallNotice(lines, orderId) {
			const lead = panel.querySelector("#recipe-shortfall-lead");
			if (lead) {
				lead.textContent = orderId
					? t("js.recipes.shortfall_with_order", { id: orderId })
					: t("js.recipes.shortfall_no_order");
			}
			shortfallNotice.hidden = true;
			shortfallError.hidden = true;
			shortfallList.innerHTML = (lines || [])
				.map(
					(s) =>
						"<li>" +
						esc(s.name) +
						" — " + t("js.recipes.missing") + " " +
						esc(String(s.missing)) +
						"</li>"
				)
				.join("");
			return new Promise((resolve) => {
				const onClose = () => {
					shortfallDialog.removeEventListener("close", onClose);
					resolve();
				};
				shortfallDialog.addEventListener("close", onClose);
				shortfallDialog.showModal();
			});
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.getAttribute("data-action") === "recipe-refresh") {
				refresh();
			}
			const toggleBtn = el.closest("[data-toggle-ingredients]");
			if (toggleBtn) {
				const id = toggleBtn.getAttribute("data-toggle-ingredients");
				const detail = list.querySelector('.recipe-ings-detail[data-ings-for="' + id + '"]');
				const chevron = toggleBtn.querySelector(".ingredient-status-btn__chevron");
				if (detail) {
					const open = detail.hasAttribute("hidden");
					if (open) {
						detail.removeAttribute("hidden");
					} else {
						detail.setAttribute("hidden", "");
					}
					toggleBtn.setAttribute("aria-expanded", open ? "true" : "false");
					if (chevron) {
						chevron.textContent = open ? "▾" : "▸";
					}
				}
				return;
			}
			if (el.getAttribute("data-action") === "recipe-new") {
				try {
					await openRecipeDialog(null);
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.getAttribute("data-action") === "recipe-remove-ing") {
				const row = el.closest(".ing-row");
				if (row) {
					row.remove();
				}
			}
			if (el.hasAttribute("data-edit-recipe")) {
				const id = el.getAttribute("data-edit-recipe");
				try {
					const recipe = await api("/api/recipes/" + id);
					await openRecipeDialog(recipe);
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.hasAttribute("data-del-recipe")) {
				const id = el.getAttribute("data-del-recipe");
				const ok = await appConfirm({
					title: t("js.recipes.delete_title"),
					message: t("js.recipes.delete_message", { id: id }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/recipes/" + id, { method: "DELETE" });
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.hasAttribute("data-hide-recipe")) {
				const id = el.getAttribute("data-hide-recipe");
				try {
					await api("/api/recipes/" + id + "/active", {
						method: "POST",
						body: JSON.stringify({ active: false }),
					});
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.hasAttribute("data-unhide-recipe")) {
				const id = el.getAttribute("data-unhide-recipe");
				try {
					await api("/api/recipes/" + id + "/active", {
						method: "POST",
						body: JSON.stringify({ active: true }),
					});
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.hasAttribute("data-deliver")) {
				const id = el.getAttribute("data-deliver");
				try {
					await api("/api/recipes/" + id + "/deliver", { method: "POST" });
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
		});

		panel.addEventListener("mousedown", (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			const suggestion = el.closest(".ing-search-results__btn");
			if (!suggestion) {
				return;
			}
			ev.preventDefault();
			const section = suggestion.closest(".recipe-ing-section");
			const id = parseInt(suggestion.getAttribute("data-item-id"), 10);
			addIngRow(panel, id);
			const search = section && section.querySelector(".ing-search");
			const results = section && section.querySelector(".ing-search-results");
			if (search) {
				search.value = "";
			}
			if (results) {
				results.hidden = true;
				results.innerHTML = "";
			}
		});

		dialog.addEventListener("close", async () => {
			brewerySel.disabled = false;
			if (dialog.returnValue !== "save") {
				return;
			}
			errEl.hidden = true;
			const fd = new FormData(form);
			const idVal = (fd.get("id") || "").toString();
			const rows = panel.querySelectorAll("#recipe-ingredients .ing-row");
			const ingredients = [];
			rows.forEach((row) => {
				const itemId = parseInt(row.querySelector('[name="item_id"]').value, 10);
				const qty = parseFloat(row.querySelector('[name="qty"]').value);
				const unit = row.querySelector('[name="unit"]').value;
				if (itemId && qty > 0) {
					ingredients.push({ inventory_item_id: itemId, qty, unit });
				}
			});
			const body = {
				brewery_id: parseInt(fd.get("brewery_id"), 10),
				name: fd.get("name"),
				ingredients,
			};
			try {
				let result;
				const isCreate = !idVal;
				if (idVal) {
					result = await api("/api/recipes/" + idVal, {
						method: "PUT",
						body: JSON.stringify(body),
					});
				} else {
					result = await api("/api/recipes", {
						method: "POST",
						body: JSON.stringify(body),
					});
				}
				form.reset();
				refresh();
				if (result && result.shortfalls && result.shortfalls.length) {
					await showShortfallNotice(result.shortfalls, result.order_id);
					refreshOrdersNavCount();
				}
				refreshBrewingNavCounts();
				if (isCreate) {
					await appInfo(pipelineGuide("recipe_created"));
				}
			} catch (e) {
				errEl.hidden = false;
				errEl.textContent = e.message;
				dialog.showModal();
			}
		});

		if (showHiddenEl) {
			showHiddenEl.addEventListener("change", () => {
				refresh();
			});
		}

		refresh();
	}

	function recipeIngCategories() {
		return [
			{ id: "malt", label: t("js.category.malt") },
			{ id: "hops", label: t("js.category.hops") },
			{ id: "yeast", label: t("js.category.yeast") },
			{ id: "misc", label: t("js.category.misc") },
			{ id: "equipment", label: t("js.category.equipment") },
		];
	}

	function unitOptions(selected) {
		const units = ["kg", "g", "L", "ml", "pcs", "pack"];
		const sel = selected || "kg";
		const list = units.includes(sel) ? units : units.concat([sel]);
		return list
			.map((u) => '<option value="' + esc(u) + '"' + (u === sel ? " selected" : "") + ">" + esc(u) + "</option>")
			.join("");
	}

	function renderIngSections(panel) {
		const wrap = panel.querySelector("#recipe-ingredients");
		if (!wrap) {
			return;
		}
		wrap.innerHTML = recipeIngCategories().map(
			(cat) =>
				'<div class="recipe-ing-section" data-category="' +
				esc(cat.id) +
				'">' +
				'<h3 class="recipe-ing-section__title">' +
				esc(cat.label) +
				"</h3>" +
				'<div class="ing-search-wrap">' +
				'<input type="search" class="ing-search" placeholder="' +
				esc(t("js.recipes.search", { category: cat.label.toLowerCase() })) +
				'" autocomplete="off">' +
				'<ul class="ing-search-results" hidden></ul>' +
				"</div>" +
				'<div class="ing-section-rows"></div>' +
				"</div>"
		).join("");

		wrap.querySelectorAll(".recipe-ing-section").forEach((section) => {
			const search = section.querySelector(".ing-search");
			const results = section.querySelector(".ing-search-results");
			const category = section.getAttribute("data-category");
			if (!search || !results) {
				return;
			}
			search.addEventListener("input", () => {
				const q = search.value.trim().toLowerCase();
				if (!q) {
					results.hidden = true;
					results.innerHTML = "";
					return;
				}
				const items = panel.querySelector("#recipe-form")._items || [];
				const matches = items
					.filter(
						(i) =>
							(i.category || "") === category &&
							String(i.name || "")
								.toLowerCase()
								.includes(q)
					)
					.slice(0, 12);
				if (!matches.length) {
					results.innerHTML = '<li class="ing-search-results__empty">' + esc(t("js.recipes.no_matches")) + "</li>";
					results.hidden = false;
					return;
				}
				results.innerHTML = matches
					.map(
						(i) =>
							'<li><button type="button" class="ing-search-results__btn" data-item-id="' +
							i.id +
							'">' +
							esc(i.name) +
							(i.producer ? " — " + esc(i.producer) : "") +
							' <span class="ing-search-results__stock">(' +
							esc(t("js.recipes.stock", { qty: i.qty, unit: i.unit || "" })) +
							")</span></button></li>"
					)
					.join("");
				results.hidden = false;
			});
			search.addEventListener("keydown", (ev) => {
				if (ev.key === "Escape") {
					results.hidden = true;
					results.innerHTML = "";
				}
			});
			search.addEventListener("blur", () => {
				setTimeout(() => {
					results.hidden = true;
				}, 150);
			});
		});
	}

	function addIngRow(panel, selectedId, qty, unit, categoryHint) {
		if (!selectedId) {
			return;
		}
		const form = panel.querySelector("#recipe-form");
		const items = (form && form._items) || [];
		const item = items.find((i) => i.id === selectedId);
		if (!item) {
			return;
		}
		const category = item.category || categoryHint || "";
		const section = panel.querySelector(
			'.recipe-ing-section[data-category="' + category + '"] .ing-section-rows'
		);
		if (!section) {
			return;
		}
		const existing = panel.querySelector(
			'#recipe-ingredients .ing-row [name="item_id"][value="' + selectedId + '"]'
		);
		if (existing) {
			return;
		}
		const rowUnit = unit || item.unit || "kg";
		const div = document.createElement("div");
		div.className = "ing-row";
		div.innerHTML =
			'<input type="hidden" name="item_id" value="' +
			item.id +
			'">' +
			'<span class="ing-row__name">' +
			esc(item.name) +
			(item.producer ? " <span class=\"ing-row__producer\">(" + esc(item.producer) + ")</span>" : "") +
			"</span>" +
			'<label>' +
			esc(t("common.qty")) +
			' <input name="qty" type="number" step="any" value="' +
			(qty != null ? qty : 1) +
			'" min="0"></label>' +
			"<label>" +
			esc(t("common.unit")) +
			' <select name="unit">' +
			unitOptions(rowUnit) +
			"</select></label>" +
			'<span class="ing-row__stock">' +
			esc(t("js.recipes.in_stock", { qty: item.qty, unit: item.unit || "" })) +
			"</span>" +
			'<button type="button" class="btn btn--small" data-action="recipe-remove-ing">' +
			esc(t("common.remove")) +
			"</button>";
		section.appendChild(div);
	}

	function recipeOptionLabel(r) {
		let label = "#" + r.id + " — " + (r.name || "");
		if (r.brewery_name) {
			label += " (" + r.brewery_name + ")";
		}
		return label;
	}

	function fillRecipeSelect(selectEl, recipes, emptyLabel) {
		if (!selectEl) {
			return;
		}
		if (!recipes || !recipes.length) {
			selectEl.innerHTML = '<option value="">' + esc(emptyLabel || t("js.recipes.no_recipes")) + "</option>";
			return;
		}
		selectEl.innerHTML = recipes
			.map((r) => '<option value="' + r.id + '">' + esc(recipeOptionLabel(r)) + "</option>")
			.join("");
	}

	async function loadSchedule(panel) {
		const list = panel.querySelector("#schedule-list");
		const form = panel.querySelector("#schedule-form");
		const errEl = panel.querySelector("#schedule-error");
		const tankSel = form.querySelector('[name="tank_id"]');
		const recipeSel = panel.querySelector("#schedule-recipe");

		async function refresh() {
			list.textContent = t("js.loading");
			try {
				const [tanks, recipes, bookings] = await Promise.all([
					api("/api/settings/tanks?active=1"),
					api("/api/recipes"),
					api("/api/schedule"),
				]);
				tankSel.innerHTML = (tanks || [])
					.map((t) => '<option value="' + t.id + '">' + esc(t.name) + "</option>")
					.join("");
				const bookable = (recipes || []).filter(
					(r) => r.status === "created" || r.status === "scheduled"
				);
				fillRecipeSelect(recipeSel, bookable, t("js.schedule.no_bookable"));
				if (!bookings || !bookings.length) {
					list.innerHTML = "<p class=\"panel__empty\">" + esc(t("js.schedule.empty")) + "</p>";
					refreshBrewingNavCounts();
					return;
				}
				list.innerHTML = table(
					[t("js.schedule.col.date"), t("js.schedule.col.end_date"), t("js.schedule.col.brewery"), t("js.schedule.col.recipe"), t("js.schedule.col.tank"), t("js.schedule.col.actions")],
					bookings
						.map((b) => {
							let actions = "";
							if (b.status === "scheduled" && b.recipe_id) {
								actions =
									'<button type="button" class="btn btn--small" data-action="schedule-unbook" data-recipe-id="' +
									b.recipe_id +
									'">' + esc(t("common.remove")) + "</button>";
							}
							return (
								"<tr><td>" +
								esc(b.date) +
								"</td><td>" +
								esc(b.end_date || b.date || "") +
								"</td><td>" +
								esc(b.brewery_name || "") +
								"</td><td>" +
								esc(b.name || "") +
								"</td><td>" +
								esc(b.tank_name || "") +
								"</td><td>" +
								actions +
								"</td></tr>"
							);
						})
						.join("")
				);
				refreshBrewingNavCounts();
			} catch (e) {
				list.textContent = e.message;
			}
		}

		panel.querySelector('[data-action="schedule-refresh"]').addEventListener("click", refresh);
		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.getAttribute("data-action") !== "schedule-unbook") {
				return;
			}
			const recipeId = el.getAttribute("data-recipe-id");
			if (!recipeId) {
				return;
			}
			const ok = await appConfirm({
				title: t("js.schedule.remove_title"),
				message: t("js.schedule.remove_message"),
			});
			if (!ok) {
				return;
			}
			try {
				await api("/api/recipes/" + recipeId + "/schedule", { method: "DELETE" });
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});
		form.addEventListener("submit", async (ev) => {
			ev.preventDefault();
			errEl.hidden = true;
			const fd = new FormData(form);
			const recipeId = fd.get("recipe_id");
			try {
				await api("/api/recipes/" + recipeId + "/schedule", {
					method: "POST",
					body: JSON.stringify({
						date: fd.get("date"),
						tank_id: parseInt(fd.get("tank_id"), 10),
						tank_days: parseInt(fd.get("tank_days"), 10) || 14,
					}),
				});
				refresh();
				await appInfo(pipelineGuide("scheduled"));
			} catch (e) {
				errEl.hidden = false;
				errEl.textContent = e.message;
			}
		});
		refresh();
	}

	async function loadBrewday(panel) {
		const list = panel.querySelector("#brewday-list");
		const form = panel.querySelector("#brewday-form");
		const errEl = panel.querySelector("#brewday-error");
		const recipeSel = panel.querySelector("#brewday-recipe");
		const ogInput = form.querySelector('[name="og"]');
		const volInput = form.querySelector('[name="brew_volume"]');
		let byID = {};

		function fillBrewdayFields(recipe) {
			ogInput.value = fmtSG(recipe && recipe.og, "1.050");
			if (recipe && recipe.brew_volume != null && recipe.brew_volume !== "") {
				volInput.value = recipe.brew_volume;
			} else {
				volInput.value = "100";
			}
			const submitBtn = form.querySelector('button[type="submit"]');
			if (submitBtn) {
				submitBtn.disabled = false;
				submitBtn.title = "";
			}
		}

		function selectRecipeForEdit(recipeId) {
			const key = String(recipeId);
			const recipe = byID[key];
			if (!recipe) {
				return;
			}
			recipeSel.value = key;
			fillBrewdayFields(recipe);
			form.scrollIntoView({ behavior: "smooth", block: "nearest" });
		}

		async function refresh(preferId) {
			list.textContent = t("js.loading");
			try {
				const recipes = await api("/api/recipes");
				const selectable = (recipes || []).filter(
					(r) =>
						r.status === "scheduled" ||
						r.status === "brewday" ||
						r.status === "hygiene_done"
				);
				byID = {};
				selectable.forEach((r) => {
					byID[String(r.id)] = r;
				});
				const prev = preferId != null ? String(preferId) : recipeSel.value;
				fillRecipeSelect(recipeSel, selectable, t("js.brewday.no_ready"));
				if (prev && byID[prev]) {
					recipeSel.value = prev;
				}
				fillBrewdayFields(byID[recipeSel.value]);
				if (!selectable.length) {
					list.innerHTML = "<p class=\"panel__empty\">" + esc(t("js.brewday.empty")) + "</p>";
					refreshBrewingNavCounts();
					return;
				}
				list.innerHTML = table(
					[t("js.brewday.col.id"), t("js.brewday.col.name"), t("js.brewday.col.brewery"), t("js.brewday.col.status"), t("js.brewday.col.og"), t("js.brewday.col.brew_vol"), ""],
					selectable
						.map((r) => {
							let actions = "";
							if (r.status === "brewday") {
								actions =
									'<button type="button" class="btn btn--small" data-action="brewday-edit" data-id="' +
									r.id +
									'">' + esc(t("common.edit")) + "</button> " +
									'<button type="button" class="btn btn--small btn--primary" data-action="brewday-hygiene-done" data-id="' +
									r.id +
									'">' + esc(t("js.brewday.mark_hygiene")) + "</button> " +
									'<button type="button" class="btn btn--small" data-action="brewday-revoke" data-id="' +
									r.id +
									'">' + esc(t("js.brewday.remove")) + "</button>";
							}
							if (r.status === "hygiene_done") {
								actions =
									'<button type="button" class="btn btn--small" data-action="brewday-edit" data-id="' +
									r.id +
									'">' + esc(t("common.edit")) + "</button> " +
									'<button type="button" class="btn btn--small" data-action="brewday-hygiene-revoke" data-id="' +
									r.id +
									'">' + esc(t("js.brewday.revoke_hygiene")) + "</button>";
							}
							return (
								"<tr><td>" +
								r.id +
								"</td><td>" +
								esc(r.name) +
								"</td><td>" +
								esc(r.brewery_name || "") +
								"</td><td>" +
								statusPill(r.status, statusText(r.status)) +
								"</td><td>" +
								fmtSG(r.og, "") +
								"</td><td>" +
								(r.brew_volume ?? "") +
								"</td><td>" +
								actions +
								"</td></tr>"
							);
						})
						.join("")
				);
				refreshBrewingNavCounts();
			} catch (e) {
				list.textContent = e.message;
			}
		}

		panel.querySelector('[data-action="brewday-refresh"]').addEventListener("click", () => {
			refresh();
		});
		recipeSel.addEventListener("change", () => {
			fillBrewdayFields(byID[recipeSel.value]);
		});
		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			const action = el.getAttribute("data-action");
			const id = el.getAttribute("data-id");
			if (action === "brewday-edit") {
				const key = String(id);
				let recipe = byID[key];
				if (!recipe) {
					return;
				}
				try {
					if (recipe.status === "hygiene_done") {
						await api("/api/recipes/" + key + "/hygiene/revoke", { method: "POST" });
						await refresh(key);
						recipe = byID[key];
					}
					selectRecipeForEdit(key);
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (action === "brewday-revoke") {
				const ok = await appConfirm({
					title: t("js.brewday.remove_title"),
					message: t("js.brewday.remove_message", { id: id }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/recipes/" + id + "/brewday/revoke", { method: "POST" });
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (action === "brewday-hygiene-revoke") {
				const ok = await appConfirm({
					title: t("js.brewday.revoke_hygiene_title"),
					message: t("js.brewday.revoke_hygiene_message", { id: id }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/recipes/" + id + "/hygiene/revoke", { method: "POST" });
					refresh(id);
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (action !== "brewday-hygiene-done") {
				return;
			}
			const ok = await appConfirm({
				title: t("js.hygiene.complete_title"),
				message: t("js.brewday.hygiene_message", { id: id }),
			});
			if (!ok) {
				return;
			}
			try {
				await api("/api/recipes/" + id + "/hygiene/complete", { method: "POST" });
				refresh();
				await appInfo(pipelineGuide("hygiene_done"));
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});
		form.addEventListener("submit", async (ev) => {
			ev.preventDefault();
			errEl.hidden = true;
			const fd = new FormData(form);
			try {
				await api("/api/recipes/" + fd.get("recipe_id") + "/brewday", {
					method: "POST",
					body: JSON.stringify({
						og: parseFloat(fd.get("og")),
						brew_volume: parseFloat(fd.get("brew_volume")),
					}),
				});
				refresh();
				await appInfo(pipelineGuide("brewday_recorded"));
			} catch (e) {
				errEl.hidden = false;
				errEl.textContent = e.message;
			}
		});
		refresh();
	}

	async function loadInventory(panel) {
		const category = panel.getAttribute("data-category");
		const isMalt = category === "malt";
		const isYeast = category === "yeast";
		const defaultUnit = category === "hops" ? "g" : category === "yeast" ? "pack" : category === "equipment" ? "pcs" : "kg";
		const list = panel.querySelector("#inventory-list");
		const dialog = panel.querySelector("#inventory-dialog");
		const form = panel.querySelector("#inventory-form");
		const titleEl = panel.querySelector("#inventory-dialog-title");
		const canEdit = canRoles(["superuser", "admin"]);
		const canDelete = canRoles(["admin"]);
		const logOffsets = new Map();
		const logPageSize = 5;
		const colCount = 10;
		const supplierSelect = form.elements.namedItem("supplier_id");
		const effectiveHint = panel.querySelector("#inventory-effective-cost");
		let suppliersById = {};

		panel.querySelectorAll(".inventory-field--malt").forEach((el) => {
			el.classList.toggle("is-role-hidden", !isMalt);
		});
		panel.querySelectorAll(".inventory-field--yeast").forEach((el) => {
			el.classList.toggle("is-role-hidden", !isYeast);
		});

		async function loadSuppliers() {
			suppliersById = {};
			if (!(supplierSelect instanceof HTMLSelectElement)) {
				return;
			}
			try {
				const suppliers = await api("/api/settings/suppliers?active=1");
				const current = supplierSelect.value;
				supplierSelect.innerHTML = '<option value="">—</option>';
				(suppliers || []).forEach((s) => {
					suppliersById[String(s.id)] = s;
					const opt = document.createElement("option");
					opt.value = String(s.id);
					opt.textContent = s.name + (s.adjust_percent ? " (" + s.adjust_percent + "%)" : "");
					supplierSelect.appendChild(opt);
				});
				if (current) {
					supplierSelect.value = current;
				}
			} catch (e) {
				/* keep empty select */
			}
		}

		function updateEffectiveHint() {
			if (!(effectiveHint instanceof HTMLElement)) {
				return;
			}
			const base = parseFloat(form.elements.namedItem("cost_price").value) || 0;
			const sid = supplierSelect instanceof HTMLSelectElement ? supplierSelect.value : "";
			const adj = sid && suppliersById[sid] ? Number(suppliersById[sid].adjust_percent) || 0 : 0;
			const effective = base * (1 + adj / 100);
			if (!sid || adj === 0) {
				effectiveHint.hidden = true;
				return;
			}
			effectiveHint.hidden = false;
			effectiveHint.textContent = t("inventory.effective_cost", { cost: String(effective), currency: currencyCode() });
		}

		function fillForm(item) {
			form.reset();
			form.elements.namedItem("id").value = item && item.id ? String(item.id) : "";
			form.elements.namedItem("name").value = (item && item.name) || "";
			form.elements.namedItem("item_type").value = (item && item.item_type) || "";
			form.elements.namedItem("producer").value = (item && item.producer) || "";
			if (supplierSelect instanceof HTMLSelectElement) {
				const sid = item && item.supplier_id != null ? String(item.supplier_id) : "";
				if (sid && !suppliersById[sid] && item.supplier_name) {
					const opt = document.createElement("option");
					opt.value = sid;
					opt.textContent = item.supplier_name;
					supplierSelect.appendChild(opt);
					suppliersById[sid] = {
						id: item.supplier_id,
						name: item.supplier_name,
						adjust_percent: item.adjust_percent || 0,
					};
				}
				supplierSelect.value = sid;
			}
			form.elements.namedItem("min_ebc").value = item && item.min_ebc != null ? item.min_ebc : 0;
			form.elements.namedItem("max_ebc").value = item && item.max_ebc != null ? item.max_ebc : 0;
			form.elements.namedItem("pitch_min_g_hl").value =
				item && item.pitch_min_g_hl != null ? item.pitch_min_g_hl : 0;
			form.elements.namedItem("pitch_max_g_hl").value =
				item && item.pitch_max_g_hl != null ? item.pitch_max_g_hl : 0;
			form.elements.namedItem("pack_size_g").value =
				item && item.pack_size_g != null ? item.pack_size_g : 0;
			form.elements.namedItem("temp_min_c").value =
				item && item.temp_min_c != null ? item.temp_min_c : 0;
			form.elements.namedItem("temp_max_c").value =
				item && item.temp_max_c != null ? item.temp_max_c : 0;
			form.elements.namedItem("link").value = (item && item.link) || "";
			form.elements.namedItem("unit").value = (item && item.unit) || defaultUnit;
			form.elements.namedItem("qty").value = item && item.qty != null ? item.qty : 0;
			form.elements.namedItem("cost_price").value = item && item.cost_price != null ? item.cost_price : 0;
			if (titleEl) {
				titleEl.textContent = item && item.id ? t("js.inventory.edit_item") : t("js.inventory.item");
			}
			updateEffectiveHint();
		}

		function bodyFromForm() {
			const fd = new FormData(form);
			const sidRaw = fd.get("supplier_id");
			const sid = sidRaw ? parseInt(String(sidRaw), 10) : 0;
			return {
				category,
				name: fd.get("name"),
				unit: fd.get("unit"),
				qty: parseFloat(fd.get("qty")) || 0,
				cost_price: parseFloat(fd.get("cost_price")) || 0,
				producer: fd.get("producer") || "",
				item_type: fd.get("item_type") || "",
				min_ebc: parseFloat(fd.get("min_ebc")) || 0,
				max_ebc: parseFloat(fd.get("max_ebc")) || 0,
				pitch_min_g_hl: parseFloat(fd.get("pitch_min_g_hl")) || 0,
				pitch_max_g_hl: parseFloat(fd.get("pitch_max_g_hl")) || 0,
				pack_size_g: parseFloat(fd.get("pack_size_g")) || 0,
				temp_min_c: parseFloat(fd.get("temp_min_c")) || 0,
				temp_max_c: parseFloat(fd.get("temp_max_c")) || 0,
				link: fd.get("link") || "",
				supplier_id: sid > 0 ? sid : null,
			};
		}

		function formatLogTime(iso) {
			if (!iso) {
				return "";
			}
			const d = new Date(iso);
			if (Number.isNaN(d.getTime())) {
				return String(iso);
			}
			return d.toLocaleString();
		}

		function formatLogUser(entry) {
			const user = entry.username || "";
			const email = entry.email || "";
			if (user && email) {
				return user + " / " + email;
			}
			return user || email || "—";
		}

		function logPanelEls(id) {
			const detail = list.querySelector('.inventory-log-detail[data-log-for="' + id + '"]');
			if (!detail) {
				return null;
			}
			return {
				detail,
				logList: detail.querySelector(".inventory-log-list"),
				moreBtn: detail.querySelector('[data-action="inventory-log-more"]'),
			};
		}

		function renderLogRows(logListEl, entries, append) {
			const rows = (entries || [])
				.map(
					(e) =>
						"<tr><td>" +
						esc(formatLogTime(e.created_at)) +
						"</td><td>" +
						esc(formatLogUser(e)) +
						"</td><td>" +
						esc(e.summary || "") +
						"</td></tr>"
				)
				.join("");
			if (append) {
				const tbody = logListEl.querySelector("tbody");
				if (tbody) {
					tbody.insertAdjacentHTML("beforeend", rows);
					return;
				}
			}
			if (!entries || !entries.length) {
				logListEl.innerHTML = "<p class=\"panel__empty\">" + esc(t("js.inventory.no_log")) + "</p>";
				return;
			}
			logListEl.innerHTML =
				'<table class="data-table data-table--nested"><thead><tr>' +
				"<th>Time</th><th>User</th><th>Change</th>" +
				"</tr></thead><tbody>" +
				rows +
				"</tbody></table>";
		}

		async function loadLogPage(id, reset) {
			const els = logPanelEls(id);
			if (!els || !els.logList) {
				return;
			}
			let offset = reset ? 0 : logOffsets.get(id) || 0;
			if (reset) {
				logOffsets.set(id, 0);
				els.logList.textContent = t("js.loading");
				if (els.moreBtn) {
					els.moreBtn.hidden = true;
				}
			}
			try {
				const data = await api(
					"/api/inventory/" + id + "/log?limit=" + logPageSize + "&offset=" + offset
				);
				const items = (data && data.items) || [];
				renderLogRows(els.logList, items, !reset && offset > 0);
				offset += items.length;
				logOffsets.set(id, offset);
				if (els.moreBtn) {
					els.moreBtn.hidden = !(data && data.has_more);
				}
			} catch (e) {
				els.logList.textContent = e.message;
				if (els.moreBtn) {
					els.moreBtn.hidden = true;
				}
			}
		}

		async function toggleLogRow(row) {
			const id = row.getAttribute("data-inventory-id");
			const els = logPanelEls(id);
			if (!els) {
				return;
			}
			const open = els.detail.hasAttribute("hidden");
			if (open) {
				els.detail.removeAttribute("hidden");
				row.setAttribute("aria-expanded", "true");
				await loadLogPage(id, true);
			} else {
				els.detail.setAttribute("hidden", "");
				row.setAttribute("aria-expanded", "false");
			}
		}

		async function refresh() {
			list.textContent = t("js.loading");
			logOffsets.clear();
			try {
				const items = await api("/api/inventory?category=" + encodeURIComponent(category));
				if (!items || !items.length) {
					list.innerHTML = "<p class=\"panel__empty\">" + esc(t("js.inventory.empty")) + "</p>";
					return;
				}
				const headers = isMalt
					? [
							t("js.inventory.col.name"),
							t("js.inventory.col.type"),
							t("js.inventory.col.producer"),
							t("js.inventory.col.supplier"),
							t("js.inventory.col.ebc"),
							t("js.inventory.col.qty"),
							t("js.inventory.col.cost"),
							t("js.inventory.col.effective_cost"),
							"",
							"",
					  ]
					: [
							t("js.inventory.col.name"),
							t("js.inventory.col.type"),
							t("js.inventory.col.producer"),
							t("js.inventory.col.supplier"),
							t("js.inventory.col.unit"),
							t("js.inventory.col.qty"),
							t("js.inventory.col.cost"),
							t("js.inventory.col.effective_cost"),
							"",
							"",
					  ];
				list.innerHTML = table(
					headers,
					items
						.map((i) => {
							const ebc =
								i.min_ebc || i.max_ebc
									? esc(String(i.min_ebc)) + "–" + esc(String(i.max_ebc))
									: "—";
							const linkCell = i.link
								? '<a href="' +
								  esc(i.link) +
								  '" target="_blank" rel="noopener noreferrer">' + esc(t("common.link")) + "</a>"
								: "";
							const orderBtn = canEdit
								? '<button type="button" class="btn btn--small" data-action="inventory-order" data-id="' +
								  i.id +
								  '" data-name="' +
								  esc(i.name) +
								  '">' + esc(t("js.inventory.add_to_order")) + "</button>"
								: "";
							const editBtn = canEdit
								? '<button type="button" class="btn btn--small" data-action="inventory-edit" data-id="' +
								  i.id +
								  '">' + esc(t("common.edit")) + "</button>"
								: "";
							const deleteBtn = canDelete
								? '<button type="button" class="btn btn--small" data-action="inventory-delete" data-id="' +
								  i.id +
								  '" data-name="' +
								  esc(i.name) +
								  '">' + esc(t("common.delete")) + "</button>"
								: "";
							const actions = [orderBtn, editBtn, deleteBtn].filter(Boolean).join(" ");
							const effective =
								i.effective_cost_price != null ? i.effective_cost_price : i.cost_price;
							const cells = isMalt
								? "<td>" +
								  esc(i.name) +
								  "</td><td>" +
								  esc(i.item_type || "") +
								  "</td><td>" +
								  esc(i.producer || "") +
								  "</td><td>" +
								  esc(i.supplier_name || "—") +
								  "</td><td>" +
								  ebc +
								  "</td><td>" +
								  i.qty +
								  "</td><td>" +
								  i.cost_price +
								  "</td><td>" +
								  effective +
								  "</td><td>" +
								  linkCell +
								  "</td><td>" +
								  actions +
								  "</td>"
								: "<td>" +
								  esc(i.name) +
								  "</td><td>" +
								  esc(i.item_type || "") +
								  "</td><td>" +
								  esc(i.producer || "") +
								  "</td><td>" +
								  esc(i.supplier_name || "—") +
								  "</td><td>" +
								  esc(i.unit) +
								  "</td><td>" +
								  i.qty +
								  "</td><td>" +
								  i.cost_price +
								  "</td><td>" +
								  effective +
								  "</td><td>" +
								  linkCell +
								  "</td><td>" +
								  actions +
								  "</td>";
							const main =
								'<tr class="inventory-row" data-inventory-id="' +
								i.id +
								'" aria-expanded="false">' +
								cells +
								"</tr>";
							const detail =
								'<tr class="inventory-log-detail" data-log-for="' +
								i.id +
								'" hidden><td colspan="' +
								colCount +
								'"><div class="inventory-log-panel"><div class="inventory-log-list"></div>' +
								'<button type="button" class="btn btn--small" data-action="inventory-log-more" data-id="' +
								i.id +
								'" hidden>' + esc(t("js.inventory.load_more")) + '</button></div></td></tr>';
							return main + detail;
						})
						.join("")
				);
			} catch (e) {
				list.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.getAttribute("data-action") === "inventory-refresh") {
				refresh();
			}
			if (el.getAttribute("data-action") === "inventory-export") {
				try {
					await downloadCSVAuth(
						"/api/inventory/export?category=" + encodeURIComponent(category),
						category + "-inventory.csv"
					);
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.getAttribute("data-action") === "inventory-import") {
				if (!canEdit) {
					return;
				}
				const input = panel.querySelector("#inventory-import-file");
				if (input) {
					input.value = "";
					input.click();
				}
				return;
			}
			if (el.getAttribute("data-action") === "inventory-new") {
				if (!canEdit) {
					return;
				}
				fillForm(null);
				dialog.showModal();
			}
			if (el.getAttribute("data-action") === "inventory-edit") {
				if (!canEdit) {
					return;
				}
				const id = parseInt(el.getAttribute("data-id"), 10);
				api("/api/inventory/" + id)
					.then((item) => {
						fillForm(item);
						dialog.showModal();
					})
					.catch((e) => { appInfo({ title: noticeTitle(), message: e.message }); });
			}
			if (el.getAttribute("data-action") === "inventory-delete") {
				if (!canDelete) {
					return;
				}
				const id = parseInt(el.getAttribute("data-id"), 10);
				const name = el.getAttribute("data-name") || "item";
				const ok = await appConfirm({
					title: t("js.inventory.delete_title"),
					message: t("js.inventory.delete_message", { name: name }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/inventory/" + id, { method: "DELETE" });
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.getAttribute("data-action") === "inventory-log-more") {
				const id = parseInt(el.getAttribute("data-id"), 10);
				loadLogPage(id, false).catch((e) => { appInfo({ title: noticeTitle(), message: e.message }); });
				return;
			}
			const invRow = el.closest(".inventory-row");
			if (invRow && !el.closest("a, button, [data-action]")) {
				toggleLogRow(invRow).catch((e) => { appInfo({ title: noticeTitle(), message: e.message }); });
				return;
			}
			if (el.getAttribute("data-action") === "inventory-order") {
				if (!canEdit) {
					return;
				}
				const id = parseInt(el.getAttribute("data-id"), 10);
				const name = el.getAttribute("data-name") || "item";
				let planning = [];
				let breweries = [];
				try {
					const [orders, breweryList] = await Promise.all([
						api("/api/inventory/orders"),
						api("/api/breweries"),
					]);
					planning = (orders || []).filter((o) => o.status === "planning");
					breweries = breweryList || [];
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
					return;
				}
				const orderOptions = [{ value: "new", label: t("js.inventory.create_order") }].concat(
					planning.map((o) => {
						const lineCount = (o.lines || []).length;
						const notes = String(o.notes || "").trim();
						const noteBit = notes ? " — " + notes.slice(0, 40) : "";
						return {
							value: String(o.id),
							label: "#" + o.id + noteBit + " (" + t("js.orders.lines_count", { n: lineCount }) + ")",
						};
					})
				);
				const breweryOptions = [{ value: "", label: t("js.inventory.unassigned") }].concat(
					breweries.map((b) => ({
						value: String(b.id),
						label: b.name || t("js.inventory.brewery_n", { id: b.id }),
					}))
				);
				const defaultBrewery = breweries.length ? String(breweries[0].id) : "";
				const values = await appPrompt({
					title: t("js.inventory.add_to_order"),
					fields: [
						{
							name: "qty",
							label: t("js.inventory.qty_to_order", { name: name }),
							type: "number",
							step: "any",
							min: "0.01",
							value: "1",
						},
						{
							name: "brewery_id",
							label: t("js.inventory.brewery"),
							type: "select",
							options: breweryOptions,
							value: defaultBrewery,
						},
						{
							name: "order_id",
							label: t("js.inventory.order"),
							type: "select",
							options: orderOptions,
							value: planning.length ? String(planning[0].id) : "new",
						},
						{
							name: "notes",
							label: t("js.inventory.notes_new"),
							type: "text",
							required: false,
						},
					],
				});
				if (!values) {
					return;
				}
				const qty = parseFloat(values.qty);
				if (!(qty > 0)) {
					await appInfo({ title: noticeTitle(), message: t("js.inventory.qty_positive") });
					return;
				}
				const breweryRaw = String(values.brewery_id || "").trim();
				const breweryID = breweryRaw ? parseInt(breweryRaw, 10) : null;
				const line = { inventory_item_id: id, qty };
				if (breweryID) {
					line.brewery_id = breweryID;
				}
				try {
					let order;
					if (values.order_id === "new") {
						order = await api("/api/inventory/orders", {
							method: "POST",
							body: JSON.stringify({
								notes: String(values.notes || "").trim(),
								lines: [line],
							}),
						});
					} else {
						order = await api("/api/inventory/orders/" + values.order_id + "/lines", {
							method: "POST",
							body: JSON.stringify(line),
						});
					}
					await appInfo({ title: noticeTitle(), message: t("js.inventory.added_to_order", { id: order.id }) });
					if (values.order_id === "new") {
						refreshOrdersNavCount();
					}
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
		});

		const importFile = panel.querySelector("#inventory-import-file");
		if (importFile) {
			importFile.addEventListener("change", async () => {
				const file = importFile.files && importFile.files[0];
				if (!file) {
					return;
				}
				try {
					const result = await importCSVAuth(
						"/api/inventory/import?category=" + encodeURIComponent(category),
						file
					);
					await appInfo({
						title: t("js.inventory.import_result"),
						message: formatImportResult(result || {}),
					});
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				importFile.value = "";
			});
		}

		dialog.addEventListener("close", async () => {
			if (dialog.returnValue !== "save") {
				return;
			}
			const idVal = form.elements.namedItem("id").value;
			const body = bodyFromForm();
			try {
				if (idVal) {
					await api("/api/inventory/" + idVal, {
						method: "PUT",
						body: JSON.stringify(body),
					});
				} else {
					await api("/api/inventory", {
						method: "POST",
						body: JSON.stringify(body),
					});
				}
				form.reset();
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});
		form.addEventListener("input", updateEffectiveHint);
		form.addEventListener("change", updateEffectiveHint);
		await loadSuppliers();
		refresh();
	}

	async function loadOrders(panel) {
		const list = panel.querySelector("#orders-list");
		const lineDialog = panel.querySelector("#order-line-dialog");
		const lineForm = panel.querySelector("#order-line-form");
		const itemSelect = panel.querySelector("#order-line-item");
		const brewerySelect = panel.querySelector("#order-line-brewery");
		const newDialog = panel.querySelector("#order-new-dialog");
		const newForm = panel.querySelector("#order-new-form");
		const confirmDialog = panel.querySelector("#order-confirm-dialog");
		const confirmForm = panel.querySelector("#order-confirm-form");
		const confirmTitle = panel.querySelector("#order-confirm-title");
		const confirmMessage = panel.querySelector("#order-confirm-message");
		const externalDialog = panel.querySelector("#order-external-dialog");
		const externalForm = panel.querySelector("#order-external-form");
		const orderedQtyDialog = panel.querySelector("#order-ordered-qty-dialog");
		const orderedQtyForm = panel.querySelector("#order-ordered-qty-form");
		const costDialog = panel.querySelector("#order-cost-dialog");
		const costForm = panel.querySelector("#order-cost-form");
		const linksDialog = panel.querySelector("#order-links-dialog");
		const linksList = panel.querySelector("#order-links-list");
		const linksTitle = panel.querySelector("#order-links-title");
		const linksOrderIdEl = panel.querySelector("#order-links-order-id");
		const canManage = canRoles(["superuser", "admin"]);
		const isAdmin = canRoles(["admin"]);

		function formatOrderDate(iso) {
			if (!iso) {
				return "";
			}
			const d = new Date(iso);
			if (Number.isNaN(d.getTime())) {
				return String(iso);
			}
			return d.toLocaleString();
		}

		function groupLinesByBrewery(lines) {
			const groups = [];
			const index = {};
			(lines || []).forEach((l) => {
				const key = l.brewery_id != null ? String(l.brewery_id) : "unassigned";
				const label = l.brewery_id != null ? l.brewery_name || t("js.inventory.brewery_n", { id: l.brewery_id }) : t("js.inventory.unassigned");
				if (index[key] == null) {
					index[key] = groups.length;
					groups.push({ key, label, lines: [] });
				}
				groups[index[key]].lines.push(l);
			});
			groups.sort((a, b) => {
				if (a.key === "unassigned") return 1;
				if (b.key === "unassigned") return -1;
				return a.label.localeCompare(b.label);
			});
			return groups;
		}

		function renderLines(order, lines) {
			if (!lines || !lines.length) {
				return "<p class=\"panel__empty\">" + esc(t("js.orders.no_lines_short")) + "</p>";
			}
			const canEditLines =
				canManage &&
				(order.status === "planning" ||
					order.status === "ordered" ||
					order.status === "paused");
			return groupLinesByBrewery(lines)
				.map((g) => {
					const items = g.lines
						.map((l) => {
							const unit = l.unit ? " " + esc(l.unit) : "";
							const orderQty =
								l.ordered_qty != null && l.ordered_qty !== undefined
									? l.ordered_qty
									: l.qty;
							const costPrice = l.cost_price != null ? l.cost_price : 0;
							const lineCost = l.line_cost != null ? l.line_cost : costPrice * orderQty;
							let text =
								esc(l.item_name) +
								" (" +
								esc(categoryText(l.category)) +
								") — " +
								t("js.orders.need") +
								" " +
								l.qty +
								unit +
								" · " +
								t("js.orders.order_qty") +
								" " +
								orderQty +
								unit +
								" · " +
								costPrice +
								" " +
								currencyCode() +
								"/unit · line " +
								lineCost +
								" " +
								currencyCode();
							if (canEditLines) {
								text +=
									' <button type="button" class="btn btn--small" data-action="order-line-ordered-qty" data-order-id="' +
									order.id +
									'" data-line-id="' +
									l.id +
									'" data-need="' +
									l.qty +
									'" data-ordered="' +
									orderQty +
									'">Set ordered qty</button>';
								text +=
									' <button type="button" class="btn btn--small" data-action="order-line-cost" data-order-id="' +
									order.id +
									'" data-line-id="' +
									l.id +
									'" data-cost="' +
									costPrice +
									'">Set cost</button>';
							}
							return "<li>" + text + "</li>";
						})
						.join("");
					return (
						'<div class="order-brewery-group"><h3 class="order-brewery-group__title">' +
						esc(g.label) +
						"</h3><ul>" +
						items +
						"</ul></div>"
					);
				})
				.join("");
		}

		function renderLinksModal(order) {
			if (linksOrderIdEl) {
				linksOrderIdEl.value = String(order.id);
			}
			if (linksTitle) {
				linksTitle.textContent = t("js.orders.product_links", { id: order.id });
			}
			const lines = order.lines || [];
			if (!lines.length) {
				linksList.innerHTML = "<p class=\"panel__empty\">" + esc(t("js.orders.no_lines")) + "</p>";
				return;
			}
			linksList.innerHTML = lines
				.map((l) => {
					const unit = l.unit ? " " + esc(l.unit) : "";
					const orderQty =
						l.ordered_qty != null && l.ordered_qty !== undefined ? l.ordered_qty : l.qty;
					const meta =
						esc(l.item_name) +
						" (" +
						esc(categoryText(l.category)) +
						") — " +
						t("js.orders.need") +
						" " +
						l.qty +
						unit +
						" · " +
						t("js.orders.order_qty") +
						" " +
						orderQty +
						unit;
					if (!l.inventory_item_id) {
						return (
							'<div class="order-link-row">' +
							'<p class="order-link-row__meta">' +
							meta +
							"</p>" +
							'<p class="panel__empty">' + esc(t("js.orders.no_catalog_item")) + "</p>" +
							"</div>"
						);
					}
					const linkVal = l.link || "";
					let actions =
						'<button type="button" class="btn btn--small" data-action="order-link-open" data-line-id="' +
						l.id +
						'">' + esc(t("js.orders.open")) + "</button>";
					if (canManage) {
						actions +=
							' <button type="button" class="btn btn--small btn--primary" data-action="order-link-save" data-line-id="' +
							l.id +
							'">' + esc(t("common.save")) + "</button>";
					}
					return (
						'<div class="order-link-row" data-line-id="' +
						l.id +
						'">' +
						'<p class="order-link-row__meta">' +
						meta +
						"</p>" +
						'<label class="order-link-row__field">' + esc(t("js.orders.product_url")) +
						'<input type="url" name="link" value="' +
						esc(linkVal) +
						'" placeholder="https://…" data-line-id="' +
						l.id +
						'">' +
						"</label>" +
						'<div class="order-link-row__actions">' +
						actions +
						"</div>" +
						"</div>"
					);
				})
				.join("");
		}

		async function openLinksModal(orderId) {
			const order = await api("/api/inventory/orders/" + orderId);
			renderLinksModal(order);
			linksDialog.showModal();
		}

		async function loadItemOptions() {
			const items = (await api("/api/inventory")) || [];
			itemSelect.innerHTML = items
				.map(
					(i) =>
						'<option value="' +
						i.id +
						'">' +
						esc(i.category) +
						" — " +
						esc(i.name) +
						(i.producer ? " (" + esc(i.producer) + ")" : "") +
						"</option>"
				)
				.join("");
		}

		async function loadBreweryOptions() {
			const breweries = (await api("/api/breweries")) || [];
			brewerySelect.innerHTML =
				'<option value="">' + esc(t("js.inventory.unassigned")) + "</option>" +
				breweries
					.map((b) => '<option value="' + b.id + '">' + esc(b.name) + "</option>")
					.join("");
		}

		async function refresh() {
			list.textContent = t("js.loading");
			try {
				const orders = await api("/api/inventory/orders");
				if (!orders || !orders.length) {
					list.innerHTML = "<p class=\"panel__empty\">" + esc(t("js.orders.empty")) + "</p>";
					refreshOrdersNavCount();
					return;
				}
				list.innerHTML = orders
					.map((o) => {
						let actions =
							'<button type="button" class="btn btn--small" data-action="order-export" data-id="' +
							o.id +
							'">' + esc(t("common.export_csv")) + "</button> " +
							'<button type="button" class="btn btn--small" data-action="order-product-links" data-id="' +
							o.id +
							'">' + esc(t("orders.product_links")) + "</button>";
						if (canManage && o.status !== "completed") {
							actions +=
								' <button type="button" class="btn btn--small" data-action="order-edit-external" data-id="' +
								o.id +
								'" data-external="' +
								esc(o.external_order_id || "") +
								'">' + esc(t("js.orders.edit_external_id")) + "</button>";
						}
						if (canManage && o.status === "planning") {
							actions +=
								' <button type="button" class="btn btn--small" data-action="order-add-line" data-id="' +
								o.id +
								'">' + esc(t("orders.add_line")) + "</button> " +
								'<button type="button" class="btn btn--small btn--primary" data-action="order-status" data-id="' +
								o.id +
								'" data-status="ordered">' + esc(t("js.orders.mark_ordered_title")) + "</button>";
						}
						if (isAdmin && o.status === "planning") {
							actions +=
								' <button type="button" class="btn btn--small" data-action="order-status" data-id="' +
								o.id +
								'" data-status="paused">' + esc(t("js.orders.pause")) + "</button>";
						}
						if (isAdmin && o.status === "paused") {
							actions +=
								' <button type="button" class="btn btn--small btn--primary" data-action="order-status" data-id="' +
								o.id +
								'" data-status="planning">' + esc(t("js.orders.resume")) + "</button>";
						}
						if (canManage && o.status === "ordered") {
							actions +=
								' <button type="button" class="btn btn--small btn--primary" data-action="order-status" data-id="' +
								o.id +
								'" data-status="completed">' + esc(t("js.orders.mark_completed")) + "</button>";
						}
						if (isAdmin && o.status !== "completed") {
							actions +=
								' <button type="button" class="btn btn--small" data-action="order-delete" data-id="' +
								o.id +
								'">' + esc(t("common.delete")) + "</button>";
						}
						const ext =
							o.external_order_id && String(o.external_order_id).trim()
								? '<p class="order-external-id">' + esc(t("js.orders.external_id_label", { id: o.external_order_id })) + "</p>"
								: '<p class="order-external-id order-external-id--empty">' + esc(t("js.orders.no_external_id")) + "</p>";
						let dates =
							'<p class="order-dates">' +
							esc(t("js.orders.created")) +
							": " +
							esc(formatOrderDate(o.created_at)) +
							" · " +
							esc(t("js.orders.updated")) +
							": " +
							esc(formatOrderDate(o.updated_at || o.created_at));
						if (o.ordered_at) {
							dates += " · " + t("js.orders.ordered") + ": " + esc(formatOrderDate(o.ordered_at));
						}
						dates += "</p>";
						const total =
							'<p class="order-total"><strong>' +
							t("js.orders.total") +
							": " +
							(o.total != null ? o.total : 0) +
							" " +
							currencyCode() +
							"</strong></p>";
						return (
							'<div class="panel__card" data-order-id="' +
							o.id +
							'"><div class="panel__card-head"><strong>#' +
							o.id +
							'</strong> ' +
							statusPill(o.status, statusText(o.status)) +
							"</div>" +
							dates +
							ext +
							(o.notes ? "<p>" + esc(o.notes) + "</p>" : "") +
							renderLines(o, o.lines) +
							total +
							'<div class="panel__card-actions">' +
							actions +
							"</div>" +
							"</div>"
						);
					})
					.join("");
				refreshOrdersNavCount();
			} catch (e) {
				list.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.getAttribute("data-action") === "orders-refresh") {
				refresh();
			}
			if (el.getAttribute("data-action") === "orders-new") {
				if (!canManage) {
					return;
				}
				newForm.reset();
				newDialog.showModal();
			}
			if (el.getAttribute("data-action") === "order-export") {
				const id = el.getAttribute("data-id");
				try {
					await downloadCSVAuth("/api/inventory/orders/" + id + "/export", "order-" + id + ".csv");
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.getAttribute("data-action") === "order-product-links") {
				try {
					await openLinksModal(el.getAttribute("data-id"));
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.getAttribute("data-action") === "order-link-open") {
				const row = el.closest(".order-link-row");
				const input = row && row.querySelector('input[name="link"]');
				const url = input ? String(input.value || "").trim() : "";
				if (!url) {
					await appInfo({ title: noticeTitle(), message: t("js.orders.no_url") });
					return;
				}
				window.open(url, "_blank", "noopener,noreferrer");
			}
			if (el.getAttribute("data-action") === "order-link-save") {
				if (!canManage) {
					return;
				}
				const lineId = el.getAttribute("data-line-id");
				const orderId = linksOrderIdEl && linksOrderIdEl.value;
				const row = el.closest(".order-link-row");
				const input = row && row.querySelector('input[name="link"]');
				if (!orderId || !lineId || !input) {
					return;
				}
				try {
					const updated = await api(
						"/api/inventory/orders/" + orderId + "/lines/" + lineId,
						{
							method: "PATCH",
							body: JSON.stringify({ link: String(input.value || "").trim() }),
						}
					);
					renderLinksModal(updated);
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.getAttribute("data-action") === "order-add-line") {
				if (!canManage) {
					return;
				}
				try {
					await loadItemOptions();
					await loadBreweryOptions();
					lineForm.elements.namedItem("order_id").value = el.getAttribute("data-id");
					lineDialog.showModal();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.getAttribute("data-action") === "order-edit-external") {
				if (!canManage) {
					return;
				}
				externalForm.elements.namedItem("order_id").value = el.getAttribute("data-id");
				externalForm.elements.namedItem("external_order_id").value =
					el.getAttribute("data-external") || "";
				externalDialog.showModal();
			}
			if (el.getAttribute("data-action") === "order-line-ordered-qty") {
				if (!canManage) {
					return;
				}
				const defaultQty = el.getAttribute("data-ordered") || el.getAttribute("data-need") || "1";
				orderedQtyForm.elements.namedItem("order_id").value = el.getAttribute("data-order-id");
				orderedQtyForm.elements.namedItem("line_id").value = el.getAttribute("data-line-id");
				orderedQtyForm.elements.namedItem("ordered_qty").value = defaultQty;
				orderedQtyDialog.showModal();
			}
			if (el.getAttribute("data-action") === "order-line-cost") {
				if (!canManage) {
					return;
				}
				costForm.elements.namedItem("order_id").value = el.getAttribute("data-order-id");
				costForm.elements.namedItem("line_id").value = el.getAttribute("data-line-id");
				costForm.elements.namedItem("cost_price").value = el.getAttribute("data-cost") || "0";
				costDialog.showModal();
			}
			if (el.getAttribute("data-action") === "order-status") {
				const status = el.getAttribute("data-status");
				const card = el.closest("[data-order-id]");
				const currentStatus =
					card && card.querySelector(".status-pill")
						? card.querySelector(".status-pill").textContent.trim()
						: "";
				const isPause = status === "paused";
				const isResume = status === "planning" && currentStatus === "paused";
				if ((isPause || isResume) && !isAdmin) {
					return;
				}
				if (!isPause && !isResume && !canManage) {
					return;
				}
				confirmForm.elements.namedItem("action").value = "status";
				confirmForm.elements.namedItem("order_id").value = el.getAttribute("data-id");
				confirmForm.elements.namedItem("status").value = status;
				if (isPause) {
					confirmTitle.textContent = t("js.orders.pause_title");
					confirmMessage.textContent = t("js.orders.pause_message");
				} else if (isResume) {
					confirmTitle.textContent = t("js.orders.resume_title");
					confirmMessage.textContent = t("js.orders.resume_message");
				} else if (status === "completed") {
					confirmTitle.textContent = t("js.orders.complete_title");
					confirmMessage.textContent = t("js.orders.complete_message");
				} else {
					confirmTitle.textContent = t("js.orders.mark_ordered_title");
					confirmMessage.textContent = t("js.orders.mark_ordered_message");
				}
				confirmDialog.showModal();
			}
			if (el.getAttribute("data-action") === "order-delete") {
				if (!isAdmin) {
					return;
				}
				confirmForm.elements.namedItem("action").value = "delete";
				confirmForm.elements.namedItem("order_id").value = el.getAttribute("data-id");
				confirmForm.elements.namedItem("status").value = "";
				confirmTitle.textContent = t("js.orders.delete_title");
				confirmMessage.textContent = t("js.orders.delete_message");
				confirmDialog.showModal();
			}
		});

		newDialog.addEventListener("close", async () => {
			if (newDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(newForm);
			try {
				const order = await api("/api/inventory/orders", {
					method: "POST",
					body: JSON.stringify({
						notes: String(fd.get("notes") || "").trim(),
						external_order_id: String(fd.get("external_order_id") || "").trim(),
						lines: [],
					}),
				});
				await appInfo({ title: noticeTitle(), message: t("js.orders.created_order", { id: order.id }) });
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		lineDialog.addEventListener("close", async () => {
			if (lineDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(lineForm);
			const orderId = fd.get("order_id");
			const breweryRaw = String(fd.get("brewery_id") || "").trim();
			const body = {
				inventory_item_id: parseInt(fd.get("inventory_item_id"), 10),
				qty: parseFloat(fd.get("qty")),
			};
			if (breweryRaw) {
				body.brewery_id = parseInt(breweryRaw, 10);
			}
			try {
				await api("/api/inventory/orders/" + orderId + "/lines", {
					method: "POST",
					body: JSON.stringify(body),
				});
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		confirmDialog.addEventListener("close", async () => {
			if (confirmDialog.returnValue !== "confirm") {
				return;
			}
			const fd = new FormData(confirmForm);
			const action = String(fd.get("action") || "");
			const orderId = fd.get("order_id");
			try {
				if (action === "delete") {
					await api("/api/inventory/orders/" + orderId, { method: "DELETE" });
				} else if (action === "status") {
					await api("/api/inventory/orders/" + orderId, {
						method: "PATCH",
						body: JSON.stringify({ status: String(fd.get("status") || "") }),
					});
				}
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		externalDialog.addEventListener("close", async () => {
			if (externalDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(externalForm);
			try {
				await api("/api/inventory/orders/" + fd.get("order_id"), {
					method: "PATCH",
					body: JSON.stringify({
						external_order_id: String(fd.get("external_order_id") || "").trim(),
					}),
				});
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		orderedQtyDialog.addEventListener("close", async () => {
			if (orderedQtyDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(orderedQtyForm);
			const orderedQty = parseFloat(fd.get("ordered_qty"));
			if (Number.isNaN(orderedQty) || orderedQty < 0) {
				await appInfo({ title: noticeTitle(), message: t("js.orders.ordered_qty_invalid") });
				return;
			}
			try {
				await api(
					"/api/inventory/orders/" + fd.get("order_id") + "/lines/" + fd.get("line_id"),
					{
						method: "PATCH",
						body: JSON.stringify({ ordered_qty: orderedQty }),
					}
				);
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		costDialog.addEventListener("close", async () => {
			if (costDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(costForm);
			const costPrice = parseFloat(fd.get("cost_price"));
			if (Number.isNaN(costPrice) || costPrice < 0) {
				await appInfo({ title: noticeTitle(), message: t("js.orders.cost_invalid") });
				return;
			}
			try {
				await api(
					"/api/inventory/orders/" + fd.get("order_id") + "/lines/" + fd.get("line_id"),
					{
						method: "PATCH",
						body: JSON.stringify({ cost_price: costPrice }),
					}
				);
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		applyRequireRoles(panel);
		refresh();
	}

	async function loadHygiene(panel) {
		const list = panel.querySelector("#hygiene-list");
		const form = panel.querySelector("#hygiene-form");
		const errEl = panel.querySelector("#hygiene-error");
		const recipeSel = panel.querySelector("#hygiene-recipe");

		function renderGroupedRoutines(routines) {
			if (!routines || !routines.length) {
				return "<p class=\"panel__empty\">" + esc(t("js.hygiene.no_routines")) + "</p>";
			}
			const groups = [];
			const index = {};
			routines.forEach((r) => {
				const section = r.description || t("js.hygiene.checklist");
				if (index[section] == null) {
					index[section] = groups.length;
					groups.push({ section, items: [] });
				}
				groups[index[section]].items.push(r);
			});
			return groups
				.map((g) => {
					const items = g.items
						.map(
							(r) =>
								"<li><span class=\"hygiene-step__id\">#" +
								r.id +
								"</span> " +
								esc(r.name) +
								"</li>"
						)
						.join("");
					return (
						'<div class="hygiene-section"><h3 class="hygiene-section__title">' +
						esc(g.section) +
						"</h3><ul class=\"hygiene-section__list\">" +
						items +
						"</ul></div>"
					);
				})
				.join("");
		}

		async function refresh() {
			list.textContent = t("js.loading");
			try {
				const [routines, recipes] = await Promise.all([
					api("/api/settings/hygiene-routines"),
					api("/api/recipes"),
				]);
				list.innerHTML = renderGroupedRoutines(routines || []);
				const brewday = (recipes || []).filter((r) => r.status === "brewday");
				fillRecipeSelect(recipeSel, brewday, t("js.hygiene.no_brewday"));
			} catch (e) {
				list.textContent = e.message;
			}
		}
		panel.querySelector('[data-action="hygiene-refresh"]').addEventListener("click", refresh);
		form.addEventListener("submit", async (ev) => {
			ev.preventDefault();
			errEl.hidden = true;
			const fd = new FormData(form);
			const recipeId = fd.get("recipe_id");
			const ok = await appConfirm({
				title: t("js.hygiene.complete_title"),
				message:
					t("js.hygiene.complete_message", { id: recipeId }),
			});
			if (!ok) {
				return;
			}
			try {
				await api("/api/recipes/" + recipeId + "/hygiene/complete", { method: "POST" });
				refresh();
				refreshBrewingNavCounts();
				await appInfo(pipelineGuide("hygiene_done"));
			} catch (e) {
				errEl.hidden = false;
				errEl.textContent = e.message;
			}
		});
		refresh();
	}

	async function loadToolsCalculators(panel) {
		// Metric yield: points·L/kg (DME ≈ 44 PPG → ~370; LME ≈ 36 PPG → ~300).
		const YIELD = { dme: 370, lme: 300 };
		/** @type {Map<string, object>} */
		const pitchYeastById = new Map();

		function yeastPitchConfigured(item) {
			return (
				item &&
				item.pitch_min_g_hl > 0 &&
				item.pitch_max_g_hl >= item.pitch_min_g_hl &&
				item.pack_size_g > 0 &&
				item.temp_max_c > 0 &&
				item.temp_max_c >= item.temp_min_c
			);
		}

		function extractYield(type) {
			return YIELD[type] || YIELD.dme;
		}

		function abvFromSG(og, fg) {
			return (og - fg) * 131.25;
		}

		function num(form, name) {
			return parseFloat(form.querySelector('[name="' + name + '"]').value);
		}

		function fmtG(g) {
			return (Math.round(g * 10) / 10).toFixed(1) + " g";
		}

		function fmtTempRange(minC, maxC) {
			return minC + "–" + maxC + " °C";
		}

		let taxCfg = { rate_sek: 2.28, free_max_abv: 2.8, discount: 1.0 };
		const taxCfgEl = panel.querySelector("#calc-tax-config");
		const yeastSelect = panel.querySelector("#calc-pitch-yeast");

		function updateABV() {
			const form = panel.querySelector("#calc-abv-form");
			const out = panel.querySelector("#calc-abv-result");
			const og = num(form, "og");
			const fg = num(form, "fg");
			if (Number.isNaN(og) || Number.isNaN(fg)) {
				out.textContent = "—";
				return;
			}
			out.textContent = abvFromSG(og, fg).toFixed(1) + " %";
		}

		function updateTax() {
			const form = panel.querySelector("#calc-tax-form");
			const og = num(form, "og");
			const fg = num(form, "fg");
			const vol = num(form, "volume");
			const abvEl = panel.querySelector("#calc-tax-abv");
			const perEl = panel.querySelector("#calc-tax-per-l");
			const totEl = panel.querySelector("#calc-tax-total");
			if (Number.isNaN(og) || Number.isNaN(fg) || Number.isNaN(vol) || vol <= 0) {
				abvEl.textContent = "—";
				perEl.textContent = "—";
				totEl.textContent = "—";
				return;
			}
			const abv = abvFromSG(og, fg);
			const perL =
				abv <= taxCfg.free_max_abv ? 0 : abv * taxCfg.rate_sek * taxCfg.discount;
			abvEl.textContent = abv.toFixed(1) + " %";
			perEl.textContent = fmtMoney(perL);
			totEl.textContent = fmtMoney(vol * perL);
		}

		function updateExtract() {
			const form = panel.querySelector("#calc-extract-form");
			const out = panel.querySelector("#calc-extract-result");
			const cur = num(form, "current_sg");
			const target = num(form, "target_sg");
			const vol = num(form, "volume");
			const type = form.querySelector('[name="extract_type"]').value;
			if (
				Number.isNaN(cur) ||
				Number.isNaN(target) ||
				Number.isNaN(vol) ||
				vol <= 0 ||
				target <= cur
			) {
				out.textContent = "—";
				return;
			}
			const points = (target - cur) * 1000;
			const kg = (points * vol) / extractYield(type);
			out.textContent = kg.toFixed(3) + " kg (" + Math.round(kg * 1000) + " g)";
		}

		function updateDilute() {
			const form = panel.querySelector("#calc-dilute-form");
			const waterEl = panel.querySelector("#calc-dilute-water");
			const finalEl = panel.querySelector("#calc-dilute-final");
			const cur = num(form, "current_sg");
			const vol = num(form, "volume");
			const wanted = num(form, "wanted_sg");
			if (
				Number.isNaN(cur) ||
				Number.isNaN(vol) ||
				Number.isNaN(wanted) ||
				vol <= 0 ||
				wanted <= 1 ||
				wanted >= cur
			) {
				waterEl.textContent = "—";
				finalEl.textContent = "—";
				return;
			}
			const finalVol = (vol * (cur - 1)) / (wanted - 1);
			const water = finalVol - vol;
			waterEl.textContent = water.toFixed(2) + " L";
			finalEl.textContent = finalVol.toFixed(2) + " L";
		}

		function clearPitchResults() {
			const infoEl = panel.querySelector("#calc-pitch-info");
			const rangeEl = panel.querySelector("#calc-pitch-range");
			const selectedEl = panel.querySelector("#calc-pitch-selected");
			const packsEl = panel.querySelector("#calc-pitch-packs");
			const tempEl = panel.querySelector("#calc-pitch-temp");
			if (infoEl) infoEl.textContent = "—";
			if (rangeEl) rangeEl.textContent = "—";
			if (selectedEl) selectedEl.textContent = "—";
			if (packsEl) packsEl.textContent = "—";
			if (tempEl) tempEl.textContent = "—";
		}

		function updatePitch() {
			const form = panel.querySelector("#calc-pitch-form");
			const infoEl = panel.querySelector("#calc-pitch-info");
			const rangeEl = panel.querySelector("#calc-pitch-range");
			const selectedEl = panel.querySelector("#calc-pitch-selected");
			const packsEl = panel.querySelector("#calc-pitch-packs");
			const tempEl = panel.querySelector("#calc-pitch-temp");
			const yeastId = form.querySelector('[name="yeast"]').value;
			const pitchLevel = form.querySelector('[name="pitch"]').value;
			const vol = num(form, "volume");
			const item = pitchYeastById.get(String(yeastId));
			if (!item) {
				clearPitchResults();
				return;
			}
			const tempC = fmtTempRange(item.temp_min_c, item.temp_max_c);
			if (infoEl) {
				infoEl.textContent =
					item.name +
					(item.item_type ? " · " + item.item_type : "") +
					" · " +
					item.pitch_min_g_hl +
					"–" +
					item.pitch_max_g_hl +
					" g/hl · pack " +
					item.pack_size_g +
					" g · " +
					tempC;
			}
			if (Number.isNaN(vol) || vol <= 0) {
				rangeEl.textContent = "—";
				selectedEl.textContent = "—";
				packsEl.textContent = "—";
				tempEl.textContent = tempC;
				return;
			}
			const hl = vol / 100;
			const minG = hl * item.pitch_min_g_hl;
			const maxG = hl * item.pitch_max_g_hl;
			const midG = (minG + maxG) / 2;
			let selectedG = midG;
			if (pitchLevel === "low") {
				selectedG = minG;
			} else if (pitchLevel === "high") {
				selectedG = maxG;
			}
			const packsExact = selectedG / item.pack_size_g;
			const packsCeil = Math.ceil(packsExact);
			rangeEl.textContent = fmtG(minG) + "–" + fmtG(maxG);
			selectedEl.textContent = fmtG(selectedG);
			packsEl.textContent =
				t("js.tools.packs", { n: packsCeil, exact: packsExact.toFixed(2) });
			tempEl.textContent = tempC;
		}

		function refreshAll() {
			updateABV();
			updateTax();
			updateExtract();
			updateDilute();
			updatePitch();
		}

		panel.addEventListener("input", (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.closest("#calc-abv-form")) {
				updateABV();
			}
			if (el.closest("#calc-tax-form")) {
				updateTax();
			}
			if (el.closest("#calc-extract-form")) {
				updateExtract();
			}
			if (el.closest("#calc-dilute-form")) {
				updateDilute();
			}
			if (el.closest("#calc-pitch-form")) {
				updatePitch();
			}
		});
		panel.addEventListener("change", (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.closest("#calc-extract-form")) {
				updateExtract();
			}
			if (el.closest("#calc-pitch-form")) {
				updatePitch();
			}
		});

		try {
			taxCfg = (await api("/api/settings/tax-config")) || taxCfg;
			if (taxCfgEl) {
				taxCfgEl.textContent =
					t("js.tools.tax_config", {
						rate: taxCfg.rate_sek,
						currency: currencyCode(),
						free: taxCfg.free_max_abv,
						discount: Math.round(taxCfg.discount * 100),
					});
			}
		} catch (e) {
			if (taxCfgEl) {
				taxCfgEl.textContent = t("js.tools.tax_config_error") + " " + e.message;
			}
		}

		if (yeastSelect) {
			try {
				const items = (await api("/api/inventory?category=yeast")) || [];
				const configured = items.filter(yeastPitchConfigured);
				pitchYeastById.clear();
				yeastSelect.innerHTML = "";
				if (configured.length === 0) {
					const opt = document.createElement("option");
					opt.value = "";
					opt.textContent = t("js.tools.no_yeast_pitch");
					opt.disabled = true;
					opt.selected = true;
					yeastSelect.appendChild(opt);
				} else {
					configured.forEach((item, i) => {
						pitchYeastById.set(String(item.id), item);
						const opt = document.createElement("option");
						opt.value = String(item.id);
						opt.textContent = item.item_type
							? item.name + " (" + item.item_type + ")"
							: item.name;
						if (i === 0) {
							opt.selected = true;
						}
						yeastSelect.appendChild(opt);
					});
				}
			} catch (e) {
				yeastSelect.innerHTML = "";
				const opt = document.createElement("option");
				opt.value = "";
				opt.textContent = t("js.tools.yeast_load_error");
				opt.disabled = true;
				opt.selected = true;
				yeastSelect.appendChild(opt);
			}
		}

		refreshAll();
	}

	async function loadEconomy(panel) {
		const examplesEl = panel.querySelector("#economy-examples");
		const form = panel.querySelector("#economy-form");
		const errEl = panel.querySelector("#economy-error");
		const exampleABVs = [3.5, 4.5, 5.0, 5.5, 6.0, 8.0];

		function renderExamples(cfg) {
			const rate = cfg.rate_sek;
			const free = cfg.free_max_abv;
			const discount = cfg.discount;
			const rows = exampleABVs
				.map((abv) => {
					const sek =
						abv <= free ? 0 : Math.round(abv * rate * discount * 100) / 100;
					return (
						"<tr><td>" +
						abv.toFixed(1) +
						" %</td><td>" +
						sek.toFixed(2) +
						" " +
						currencyPerLiter() +
						"</td></tr>"
					);
				})
				.join("");
			examplesEl.innerHTML =
				"<p class=\"panel__lead\">" +
				esc(t("js.economy.examples_lead", { rate: rate, discount: discount })) +
				"</p>" +
				table([t("js.economy.examples_abv"), t("js.economy.examples_tax")], rows);
		}

		async function refresh() {
			examplesEl.textContent = t("js.loading");
			try {
				const cfg = await api("/api/settings/tax-config");
				form.querySelector('[name="rate_sek"]').value = cfg.rate_sek;
				form.querySelector('[name="free_max_abv"]').value = cfg.free_max_abv;
				form.querySelector('[name="discount"]').value = String(cfg.discount);
				renderExamples(cfg);
			} catch (e) {
				examplesEl.textContent = e.message;
			}
		}

		panel.querySelector('[data-action="economy-refresh"]').addEventListener("click", refresh);
		form.addEventListener("submit", async (ev) => {
			ev.preventDefault();
			errEl.hidden = true;
			if (!canRoles(["superuser", "admin"])) {
				return;
			}
			const fd = new FormData(form);
			try {
				const cfg = await api("/api/settings/tax-config", {
					method: "PUT",
					body: JSON.stringify({
						rate_sek: parseFloat(fd.get("rate_sek")),
						free_max_abv: parseFloat(fd.get("free_max_abv")),
						discount: parseFloat(fd.get("discount")),
					}),
				});
				renderExamples(cfg);
			} catch (e) {
				errEl.hidden = false;
				errEl.textContent = e.message;
			}
		});
		form.addEventListener("change", () => {
			const rate = parseFloat(form.querySelector('[name="rate_sek"]').value);
			const free = parseFloat(form.querySelector('[name="free_max_abv"]').value);
			const discount = parseFloat(form.querySelector('[name="discount"]').value);
			if (!Number.isNaN(rate) && !Number.isNaN(free) && !Number.isNaN(discount)) {
				renderExamples({ rate_sek: rate, free_max_abv: free, discount: discount });
			}
		});
		refresh();
	}

	async function loadEconomyDeliveries(panel) {
		const list = panel.querySelector("#economy-deliveries-list");
		const monthInput = panel.querySelector("#economy-deliveries-month");
		const sumVol = panel.querySelector("#economy-deliveries-sum-vol");
		const sumCost = panel.querySelector("#economy-deliveries-sum-cost");
		const sumTax = panel.querySelector("#economy-deliveries-sum-tax");
		const sumNet = panel.querySelector("#economy-deliveries-sum-net");
		const sumProfit = panel.querySelector("#economy-deliveries-sum-profit");
		let rows = [];

		const columnHelp = {
			delivered: {
				title: t("js.economy.col.delivered"),
				message: t("js.economy.help.delivered"),
			},
			id: {
				title: t("js.economy.col.id"),
				message: t("js.economy.help.id"),
			},
			name: {
				title: t("js.economy.col.name"),
				message: t("js.economy.help.name"),
			},
			brewery: {
				title: t("js.economy.col.brewery"),
				message: t("js.economy.help.brewery"),
			},
			abv: {
				title: t("js.economy.col.abv"),
				message: t("js.economy.help.abv"),
			},
			volume: {
				title: t("js.economy.col.volume"),
				message: t("js.economy.help.volume"),
			},
			cost: {
				title: t("js.economy.col.cost"),
				message: t("js.economy.help.cost"),
			},
			tax: {
				title: t("js.economy.col.tax"),
				message: t("js.economy.help.tax"),
			},
			net: {
				title: t("js.economy.col.net"),
				message: t("js.economy.help.net"),
			},
			profit: {
				title: t("js.economy.col.profit"),
				message: t("js.economy.help.profit"),
			},
			"net-per-l": {
				title: t("js.economy.net_per_l", { currency: currencyCode() }),
				message: t("js.economy.net_per_l_help"),
			},
		};

		const helpHeaders = [
			{ key: "delivered", label: t("js.economy.col.delivered") },
			{ key: "id", label: t("js.economy.col.id") },
			{ key: "name", label: t("js.economy.col.name") },
			{ key: "brewery", label: t("js.economy.col.brewery") },
			{ key: "abv", label: t("js.economy.col.abv") },
			{ key: "volume", label: t("js.economy.col.volume") },
			{ key: "cost", label: t("js.economy.col.cost") },
			{ key: "tax", label: t("js.economy.col.tax") },
			{ key: "net", label: t("js.economy.col.net") },
			{ key: "profit", label: t("js.economy.col.profit") },
			{ key: "net-per-l", label: t("js.economy.net_per_l", { currency: currencyCode() }) },
		];

		function helpTable(rowsHtml) {
			return (
				'<div class="table-scroll"><table class="data-table"><thead><tr>' +
				helpHeaders
					.map(
						(h) =>
							'<th class="data-table__th--help" tabindex="0" data-col-help="' +
							esc(h.key) +
							'">' +
							esc(h.label) +
							"</th>"
					)
					.join("") +
				"</tr></thead><tbody>" +
				rowsHtml +
				"</tbody></table></div>"
			);
		}

		function currentMonthValue() {
			const d = new Date();
			const y = d.getFullYear();
			const m = String(d.getMonth() + 1).padStart(2, "0");
			return y + "-" + m;
		}

		function netPerLiter(r) {
			const vol = Number(r.delivery_volume);
			const net = Number(r.net);
			if (!vol || Number.isNaN(vol) || vol <= 0 || Number.isNaN(net)) {
				return "";
			}
			return fmtMoney(net / vol);
		}

		function setSummary(totals) {
			if (!totals) {
				sumVol.textContent = "—";
				sumCost.textContent = "—";
				sumNet.textContent = "—";
				sumTax.textContent = "—";
				sumProfit.textContent = "—";
				return;
			}
			sumVol.textContent = fmtMoneyAmount(totals.volume) + " L";
			sumCost.textContent = fmtMoney(totals.cost);
			sumTax.textContent = fmtMoney(totals.tax);
			sumNet.textContent = fmtMoney(totals.net);
			sumProfit.textContent = fmtMoney(totals.profit);
		}

		function csvEscape(v) {
			const s = v == null ? "" : String(v);
			if (/[",\n\r]/.test(s)) {
				return '"' + s.replace(/"/g, '""') + '"';
			}
			return s;
		}

		function downloadCSV() {
			const month = monthInput.value || currentMonthValue();
			const header = [
				t("js.economy.col.delivered"),
				t("js.economy.col.id"),
				t("js.economy.col.name"),
				t("js.economy.col.brewery"),
				t("js.economy.col.abv") + " %",
				t("js.economy.col.volume"),
				t("js.economy.col.cost"),
				t("js.economy.col.tax"),
				t("js.economy.col.net"),
				t("js.economy.col.profit"),
				t("js.economy.net_per_l", { currency: currencyCode() }),
			];
			const lines = [header.join(",")];
			rows.forEach((r) => {
				const abv =
					r.og != null && r.fg != null
						? ((r.og - r.fg) * 131.25).toFixed(1)
						: "";
				const cost = r.cost != null ? Number(r.cost) : NaN;
				const net = r.net != null ? Number(r.net) : NaN;
				const profit =
					!Number.isNaN(cost) && !Number.isNaN(net) ? (net - cost).toFixed(2) : "";
				lines.push(
					[
						datePart(r.delivered_at),
						r.id,
						r.name,
						r.brewery_name || "",
						abv,
						r.delivery_volume != null ? r.delivery_volume : "",
						r.cost != null ? Number(r.cost).toFixed(2) : "",
						r.tax != null ? Number(r.tax).toFixed(2) : "",
						r.net != null ? Number(r.net).toFixed(2) : "",
						profit,
						netPerLiter(r),
					]
						.map(csvEscape)
						.join(",")
				);
			});
			const blob = new Blob([lines.join("\r\n")], { type: "text/csv;charset=utf-8" });
			const url = URL.createObjectURL(blob);
			const a = document.createElement("a");
			a.href = url;
			a.download = "deliveries-" + month + ".csv";
			document.body.appendChild(a);
			a.click();
			a.remove();
			URL.revokeObjectURL(url);
		}

		async function refresh() {
			const month = monthInput.value || currentMonthValue();
			monthInput.value = month;
			list.textContent = t("js.loading");
			setSummary(null);
			rows = [];
			try {
				const recipes = await api("/api/recipes?include_hidden=1");
				rows = (recipes || [])
					.filter(
						(r) =>
							r.status === "delivered" &&
							r.delivered_at &&
							String(r.delivered_at).slice(0, 7) === month
					)
					.sort((a, b) =>
						String(a.delivered_at).localeCompare(String(b.delivered_at))
					);
				const totals = { volume: 0, cost: 0, tax: 0, net: 0, profit: 0 };
				rows.forEach((r) => {
					const cost = Number(r.cost) || 0;
					const net = Number(r.net) || 0;
					totals.volume += Number(r.delivery_volume) || 0;
					totals.cost += cost;
					totals.tax += Number(r.tax) || 0;
					totals.net += net;
					totals.profit += net - cost;
				});
				setSummary(totals);
				if (!rows.length) {
					list.innerHTML =
						'<p class="panel__empty">' + esc(t("js.economy.no_deliveries", { month: month })) + "</p>";
					return;
				}
				list.innerHTML = helpTable(
					rows
						.map((r) => {
							const cost = Number(r.cost) || 0;
							const net = Number(r.net) || 0;
							return (
								"<tr><td>" +
								esc(datePart(r.delivered_at)) +
								"</td><td>" +
								r.id +
								"</td><td>" +
								esc(r.name) +
								"</td><td>" +
								esc(r.brewery_name || "") +
								"</td><td>" +
								esc(recipeABV(r)) +
								"</td><td>" +
								fmtMoneyAmount(r.delivery_volume) +
								"</td><td>" +
								fmtMoney(r.cost) +
								"</td><td>" +
								fmtMoney(r.tax) +
								"</td><td>" +
								fmtMoney(r.net) +
								"</td><td>" +
								fmtMoney(net - cost) +
								"</td><td>" +
								esc(netPerLiter(r)) +
								"</td></tr>"
							);
						})
						.join("")
				);
			} catch (e) {
				list.textContent = e.message;
			}
		}

		if (!monthInput.value) {
			monthInput.value = currentMonthValue();
		}
		panel
			.querySelector('[data-action="economy-deliveries-refresh"]')
			.addEventListener("click", refresh);
		panel
			.querySelector('[data-action="economy-deliveries-csv"]')
			.addEventListener("click", downloadCSV);
		monthInput.addEventListener("change", refresh);
		list.addEventListener("click", (ev) => {
			const th = ev.target.closest("[data-col-help]");
			if (!th || !list.contains(th)) {
				return;
			}
			const key = th.getAttribute("data-col-help");
			const help = columnHelp[key];
			if (!help) {
				return;
			}
			appInfo(help);
		});
		list.addEventListener("keydown", (ev) => {
			if (ev.key !== "Enter" && ev.key !== " ") {
				return;
			}
			const th = ev.target.closest("[data-col-help]");
			if (!th || !list.contains(th)) {
				return;
			}
			ev.preventDefault();
			const key = th.getAttribute("data-col-help");
			const help = columnHelp[key];
			if (help) {
				appInfo(help);
			}
		});
		refresh();
	}

	async function loadDelivery(panel) {
		const list = panel.querySelector("#delivery-list");
		const form = panel.querySelector("#delivery-form");
		const errEl = panel.querySelector("#delivery-error");
		const recipeSel = panel.querySelector("#delivery-recipe");
		const multSel = panel.querySelector("#delivery-multiplier");
		const beerNetInput = panel.querySelector("#delivery-beer-net");
		const fgInput = form.querySelector('[name="fg"]');
		const volInput = form.querySelector('[name="delivery_volume"]');
		const previewABV = panel.querySelector("#delivery-preview-abv");
		const previewCost = panel.querySelector("#delivery-preview-cost");
		const previewTax = panel.querySelector("#delivery-preview-tax");
		const previewNet = panel.querySelector("#delivery-preview-net");
		const previewProfit = panel.querySelector("#delivery-preview-profit");
		const previewPerL = panel.querySelector("#delivery-preview-per-l");
		let byID = {};
		let taxCfg = { rate_sek: 2.28, free_max_abv: 2.8, discount: 1.0 };
		let previewOG = null;
		let previewCostTotal = 0;

		function fillFG(recipe) {
			fgInput.value = fmtSG(recipe && recipe.fg, "1.010");
		}

		function clearPreview() {
			previewABV.textContent = "—";
			previewCost.textContent = "—";
			previewTax.textContent = "—";
			previewNet.textContent = "—";
			previewProfit.textContent = "—";
			previewPerL.textContent = "—";
		}

		function selectedMultiplier() {
			const opt = multSel.selectedOptions[0];
			if (!opt) {
				return NaN;
			}
			const raw = opt.getAttribute("data-multiplier");
			const n = parseFloat(raw);
			return Number.isNaN(n) ? NaN : n;
		}

		function updatePreview() {
			const og = previewOG;
			const fg = parseFloat(fgInput.value);
			const vol = parseFloat(volInput.value);
			const beerNet = parseFloat(beerNetInput.value);
			const mult = selectedMultiplier();
			if (
				og == null ||
				Number.isNaN(og) ||
				Number.isNaN(fg) ||
				Number.isNaN(vol) ||
				vol <= 0 ||
				Number.isNaN(beerNet) ||
				Number.isNaN(mult)
			) {
				clearPreview();
				return;
			}
			const abv = (og - fg) * 131.25;
			const taxPerL =
				abv <= taxCfg.free_max_abv ? 0 : abv * taxCfg.rate_sek * taxCfg.discount;
			const tax = vol * taxPerL;
			const cost = previewCostTotal;
			const perL = beerNet * mult;
			const net = perL * vol;
			previewABV.textContent = abv.toFixed(1) + " %";
			previewCost.textContent = fmtMoney(cost);
			previewTax.textContent = fmtMoney(tax);
			previewNet.textContent = fmtMoney(net);
			previewProfit.textContent = fmtMoney(net - cost);
			previewPerL.textContent = fmtMoney(perL);
		}

		async function loadRecipePreview(id) {
			previewOG = null;
			previewCostTotal = 0;
			if (!id) {
				clearPreview();
				return;
			}
			try {
				const recipe = await api("/api/recipes/" + id);
				previewOG = recipe.og != null ? Number(recipe.og) : null;
				const ings = recipe.ingredients || [];
				previewCostTotal = ings.reduce(
					(sum, ing) => sum + Number(ing.qty || 0) * Number(ing.cost_price || 0),
					0
				);
				if (recipe.delivery_volume != null && recipe.delivery_volume > 0) {
					volInput.value = recipe.delivery_volume;
				}
				updatePreview();
			} catch (e) {
				clearPreview();
			}
		}

		async function loadPricingControls() {
			const [mults, beer, tax] = await Promise.all([
				api("/api/settings/multipliers?active=1"),
				api("/api/settings/beer-price"),
				api("/api/settings/tax-config"),
			]);
			taxCfg = tax || taxCfg;
			const listMults = mults || [];
			multSel.innerHTML = listMults
				.map((m) => {
					const selected = m.name === "default" ? " selected" : "";
					return (
						'<option value="' +
						m.id +
						'" data-multiplier="' +
						m.multiplier +
						'"' +
						selected +
						">" +
						esc(m.name) +
						" (×" +
						m.multiplier +
						")</option>"
					);
				})
				.join("");
			if (listMults.length && !multSel.value) {
				multSel.value = String(listMults[0].id);
			}
			beerNetInput.value = beer.min_net_sek_per_liter ?? 0;
		}

		async function refresh() {
			try {
				const recipes = await api("/api/recipes");
				const selectable = (recipes || []).filter(
					(r) => r.status === "hygiene_done" || r.status === "ready_for_delivery"
				);
				byID = {};
				selectable.forEach((r) => {
					byID[String(r.id)] = r;
				});
				const prev = recipeSel.value;
				fillRecipeSelect(recipeSel, selectable, t("js.delivery.no_ready"));
				if (prev && byID[prev]) {
					recipeSel.value = prev;
				}
				fillFG(byID[recipeSel.value]);
				await loadRecipePreview(recipeSel.value);
				const relevant = (recipes || []).filter((r) =>
					["hygiene_done", "ready_for_delivery", "delivered"].includes(r.status)
				);
				if (!relevant.length) {
					list.innerHTML = "<p class=\"panel__empty\">" + esc(t("js.delivery.empty")) + "</p>";
					refreshBrewingNavCounts();
					return;
				}
				list.innerHTML = table(
					[
						t("js.delivery.col.id"),
						t("js.delivery.col.name"),
						t("js.delivery.col.brewery"),
						t("js.delivery.col.status"),
						t("js.delivery.col.brew_date"),
						t("js.delivery.col.delivery_date"),
						t("js.delivery.col.abv"),
						t("js.delivery.col.cost"),
						t("js.delivery.col.tax"),
						t("js.delivery.col.net"),
						t("js.delivery.col.profit"),
						"",
					],
					relevant
						.map((r) => {
							let btn = "";
							if (r.status === "ready_for_delivery") {
								btn =
									'<button type="button" class="btn btn--small" data-deliver="' +
									r.id +
									'">' + esc(t("js.recipes.deliver")) + "</button>";
							} else if (r.status === "delivered") {
								btn =
									'<button type="button" class="btn btn--small" data-revoke-delivery="' +
									r.id +
									'">' + esc(t("js.delivery.revoke")) + "</button>";
							}
							const hasCostNet = r.cost != null && r.net != null;
							const profit = hasCostNet
								? fmtMoney((Number(r.net) || 0) - (Number(r.cost) || 0))
								: "—";
							return (
								"<tr><td>" +
								r.id +
								"</td><td>" +
								esc(r.name) +
								"</td><td>" +
								esc(r.brewery_name || "") +
								"</td><td>" +
								statusPill(r.status, statusText(r.status)) +
								"</td><td>" +
								esc(r.booked_date || "") +
								"</td><td>" +
								esc(datePart(r.delivered_at)) +
								"</td><td>" +
								esc(recipeABV(r)) +
								"</td><td>" +
								fmtMoney(r.cost) +
								"</td><td>" +
								fmtMoney(r.tax) +
								"</td><td>" +
								fmtMoney(r.net) +
								"</td><td>" +
								profit +
								"</td><td>" +
								btn +
								"</td></tr>"
							);
						})
						.join("")
				);
				refreshBrewingNavCounts();
			} catch (e) {
				list.textContent = e.message;
			}
		}
		panel.querySelector('[data-action="delivery-refresh"]').addEventListener("click", refresh);
		recipeSel.addEventListener("change", async () => {
			fillFG(byID[recipeSel.value]);
			await loadRecipePreview(recipeSel.value);
		});
		["input", "change"].forEach((evt) => {
			fgInput.addEventListener(evt, updatePreview);
			volInput.addEventListener(evt, updatePreview);
			beerNetInput.addEventListener(evt, updatePreview);
			multSel.addEventListener(evt, updatePreview);
		});
		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.hasAttribute("data-deliver")) {
				try {
					await api("/api/recipes/" + el.getAttribute("data-deliver") + "/deliver", {
						method: "POST",
					});
					refresh();
					await appInfo(pipelineGuide("delivered"));
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.hasAttribute("data-revoke-delivery")) {
				const id = el.getAttribute("data-revoke-delivery");
				const ok = await appConfirm({
					title: t("js.delivery.revoke_title"),
					message: t("js.delivery.revoke_message", { id: id }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/recipes/" + id + "/delivery/revoke", { method: "POST" });
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
		});
		form.addEventListener("submit", async (ev) => {
			ev.preventDefault();
			errEl.hidden = true;
			const fd = new FormData(form);
			try {
				await api("/api/recipes/" + fd.get("recipe_id") + "/delivery", {
					method: "POST",
					body: JSON.stringify({
						fg: parseFloat(fd.get("fg")),
						delivery_volume: parseFloat(fd.get("delivery_volume")),
						beer_net_sek_per_liter: parseFloat(fd.get("beer_net_sek_per_liter")),
						multiplier_id: parseInt(fd.get("multiplier_id"), 10),
					}),
				});
				refresh();
				await appInfo(pipelineGuide("ready_for_delivery"));
			} catch (e) {
				errEl.hidden = false;
				errEl.textContent = e.message;
			}
		});
		try {
			await loadPricingControls();
		} catch (e) {
			errEl.hidden = false;
			errEl.textContent = e.message;
		}
		refresh();
	}

	async function loadIAMUsers(panel) {
		const usersEl = panel.querySelector("#iam-users");

		async function refreshUsers() {
			try {
				const users = await api("/api/users");
				usersEl.innerHTML = table(
					[t("common.id"), t("common.username"), t("common.name"), t("common.email"), t("common.role"), t("common.active"), ""],
					(users || [])
						.map((u) => {
							const active = u.active !== false;
							const label = active ? t("js.iam.active") : t("js.iam.inactive");
							const btnLabel = active ? t("js.iam.deactivate") : t("js.iam.activate");
							const name = [u.first_name, u.last_name].filter(Boolean).join(" ");
							const btn =
								'<button type="button" class="btn btn--small" data-edit-user="' +
								u.id +
								'">' + esc(t("common.edit")) + "</button> " +
								'<button type="button" class="btn btn--small" data-set-active="' +
								u.id +
								'" data-active="' +
								(active ? "0" : "1") +
								'">' +
								btnLabel +
								'</button> <button type="button" class="btn btn--small" data-reset-password="' +
								u.id +
								'">' + esc(t("iam.users.reset_password")) + "</button>";
							return (
								"<tr><td>" +
								u.id +
								"</td><td>" +
								esc(u.username) +
								"</td><td>" +
								esc(name || "—") +
								"</td><td>" +
								esc(u.email || "") +
								"</td><td>" +
								esc(u.role || "—") +
								"</td><td>" +
								label +
								"</td><td>" +
								btn +
								"</td></tr>"
							);
						})
						.join("")
				);
			} catch (e) {
				usersEl.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			const action = el.getAttribute("data-action");
			if (action === "iam-users-refresh") {
				refreshUsers();
			}
			if (action === "iam-users-export") {
				try {
					await downloadCSVAuth("/api/users/export", "users.csv");
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (action === "iam-users-import") {
				const input = panel.querySelector("#iam-users-import-file");
				if (input) {
					input.value = "";
					input.click();
				}
			}
			if (action === "iam-user-new") {
				const dlg = panel.querySelector("#iam-user-dialog");
				const brewSel = dlg.querySelector('[name="brewery_id"]');
				try {
					const breweries = await api("/api/breweries");
					brewSel.innerHTML =
						'<option value="">' + esc(t("common.none")) + "</option>" +
						(breweries || [])
							.map((b) => '<option value="' + b.id + '">' + esc(b.name) + "</option>")
							.join("");
				} catch (e) {
					brewSel.innerHTML = '<option value="">' + esc(t("common.none")) + "</option>";
				}
				dlg.showModal();
			}
			if (el.hasAttribute("data-edit-user")) {
				const id = el.getAttribute("data-edit-user");
				const dlg = panel.querySelector("#iam-edit-user-dialog");
				const form = panel.querySelector("#iam-edit-user-form");
				const roleSel = form.querySelector('[name="role"]');
				const hint = panel.querySelector("#iam-edit-role-hint");
				const selfID = sessionStorage.getItem("brewhouse_user_id") || "";
				try {
					const user = await api("/api/users/" + id);
					form.querySelector('[name="user_id"]').value = user.id;
					form.querySelector('[name="username"]').value = user.username || "";
					form.querySelector('[name="email"]').value = user.email || "";
					form.querySelector('[name="first_name"]').value = user.first_name || "";
					form.querySelector('[name="last_name"]').value = user.last_name || "";
					form.querySelector('[name="address_line1"]').value = user.address_line1 || "";
					form.querySelector('[name="address_line2"]').value = user.address_line2 || "";
					form.querySelector('[name="phone"]').value = user.phone || "";
					form.querySelector('[name="instagram"]').value = user.instagram || "";
					roleSel.value = user.role || "user";
					const editingSelfAdmin =
						String(user.id) === selfID && (user.role || "") === "admin";
					form.querySelector('[name="username"]').readOnly = false;
					roleSel.disabled = editingSelfAdmin;
					hint.hidden = !editingSelfAdmin;
					dlg.showModal();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.hasAttribute("data-set-active")) {
				const id = el.getAttribute("data-set-active");
				const active = el.getAttribute("data-active") === "1";
				try {
					await api("/api/users/" + id + "/active", {
						method: "PATCH",
						body: JSON.stringify({ active }),
					});
					refreshUsers();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.hasAttribute("data-reset-password")) {
				const id = el.getAttribute("data-reset-password");
				const dlg = panel.querySelector("#iam-reset-password-dialog");
				const form = panel.querySelector("#iam-reset-password-form");
				try {
					const user = await api("/api/users/" + id);
					form.querySelector('[name="user_id"]').value = user.id;
					form.querySelector('[name="username"]').value = user.username || "";
					form.querySelector('[name="email"]').value = user.email || "";
					form.querySelector('[name="role"]').value = user.role || "user";
					form.querySelector('[name="first_name"]').value = user.first_name || "";
					form.querySelector('[name="last_name"]').value = user.last_name || "";
					form.querySelector('[name="address_line1"]').value = user.address_line1 || "";
					form.querySelector('[name="address_line2"]').value = user.address_line2 || "";
					form.querySelector('[name="phone"]').value = user.phone || "";
					form.querySelector('[name="instagram"]').value = user.instagram || "";
					form.querySelector('[name="password"]').value = "";
					dlg.showModal();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
		});

		const userDialog = panel.querySelector("#iam-user-dialog");
		const userForm = panel.querySelector("#iam-user-form");
		userDialog.addEventListener("close", async () => {
			if (userDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(userForm);
			const password = String(fd.get("password") || "");
			if (!isPasswordComplex(password)) {
				await appInfo({ title: noticeTitle(), message: t("common.password_invalid") });
				return;
			}
			const body = {
				username: fd.get("username"),
				password: password,
				email: fd.get("email"),
				first_name: fd.get("first_name"),
				last_name: fd.get("last_name"),
				address_line1: fd.get("address_line1"),
				address_line2: fd.get("address_line2"),
				phone: fd.get("phone"),
				instagram: fd.get("instagram"),
				role: fd.get("role"),
			};
			const breweryID = fd.get("brewery_id");
			if (breweryID) {
				body.brewery_id = parseInt(breweryID, 10);
			}
			try {
				await api("/api/users", {
					method: "POST",
					body: JSON.stringify(body),
				});
				userForm.reset();
				refreshUsers();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		const editUserDialog = panel.querySelector("#iam-edit-user-dialog");
		const editUserForm = panel.querySelector("#iam-edit-user-form");
		editUserDialog.addEventListener("close", async () => {
			if (editUserDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(editUserForm);
			const id = fd.get("user_id");
			const roleSel = editUserForm.querySelector('[name="role"]');
			const role = roleSel.disabled ? "admin" : fd.get("role");
			const selfID = sessionStorage.getItem("brewhouse_user_id") || "";
			if (String(id) === selfID && role !== "admin") {
				await appInfo({ title: noticeTitle(), message: t("js.iam.cannot_demote_self") });
				return;
			}
			try {
				await api("/api/users/" + id, {
					method: "PATCH",
					body: JSON.stringify({
						username: fd.get("username"),
						email: fd.get("email"),
						first_name: fd.get("first_name"),
						last_name: fd.get("last_name"),
						address_line1: fd.get("address_line1"),
						address_line2: fd.get("address_line2"),
						phone: fd.get("phone"),
						instagram: fd.get("instagram"),
						role: role,
					}),
				});
				editUserForm.reset();
				roleSel.disabled = false;
				panel.querySelector("#iam-edit-role-hint").hidden = true;
				refreshUsers();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		const resetDialog = panel.querySelector("#iam-reset-password-dialog");
		const resetForm = panel.querySelector("#iam-reset-password-form");
		resetDialog.addEventListener("close", async () => {
			if (resetDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(resetForm);
			const id = fd.get("user_id");
			const password = String(fd.get("password") || "");
			if (!isPasswordComplex(password)) {
				await appInfo({ title: noticeTitle(), message: t("common.password_invalid") });
				return;
			}
			try {
				await api("/api/users/" + id, {
					method: "PATCH",
					body: JSON.stringify({
						username: fd.get("username"),
						email: fd.get("email"),
						first_name: fd.get("first_name"),
						last_name: fd.get("last_name"),
						address_line1: fd.get("address_line1"),
						address_line2: fd.get("address_line2"),
						phone: fd.get("phone"),
						instagram: fd.get("instagram"),
						role: fd.get("role"),
						password: password,
					}),
				});
				resetForm.reset();
				await appInfo({ title: noticeTitle(), message: t("js.iam.password_updated") });
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		const usersImportFile = panel.querySelector("#iam-users-import-file");
		if (usersImportFile) {
			usersImportFile.addEventListener("change", async () => {
				const file = usersImportFile.files && usersImportFile.files[0];
				if (!file) {
					return;
				}
				try {
					const result = await importCSVAuth("/api/users/import", file);
					await appInfo({
						title: t("js.inventory.import_result"),
						message: formatImportResult(result || {}),
					});
					refreshUsers();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				} finally {
					usersImportFile.value = "";
				}
			});
		}

		refreshUsers();
	}

	async function loadIAMBreweries(panel) {
		const brewEl = panel.querySelector("#iam-breweries");
		const isAdmin = canRoles("admin");
		applyRequireRoles(panel);

		function breweryLogoURL(id) {
			return "/brewery/" + id + "/logo?t=" + Date.now();
		}

		function setBreweryLogoPreview(breweryID, configured) {
			const preview = panel.querySelector("#iam-brewery-logo-preview");
			if (!preview) {
				return;
			}
			if (configured && breweryID) {
				preview.hidden = false;
				preview.src = breweryLogoURL(breweryID);
			} else {
				preview.hidden = true;
				preview.removeAttribute("src");
			}
		}

		async function refreshBreweries() {
			try {
				const list = await api("/api/breweries");
				brewEl.innerHTML = (list || [])
					.map((b) => {
						const logo = b.logo_configured
							? '<img class="iam-brewery-card__logo" src="' +
								breweryLogoURL(b.id) +
								'" alt="" width="48" height="48">'
							: "";
						const editBtn = b.can_manage
							? '<button type="button" class="btn btn--small" data-edit-brewery="' +
								b.id +
								'">' + esc(t("common.edit")) + "</button> "
							: "";
						const delBtn = isAdmin
							? '<button type="button" class="btn btn--small" data-del-brewery="' +
								b.id +
								'" data-brewery-name="' +
								esc(b.name) +
								'">' + esc(t("common.remove")) + "</button> "
							: "";
						const addBtn = b.can_manage
							? '<button type="button" class="btn btn--small" data-add-member="' +
								b.id +
								'" data-brewery-name="' +
								esc(b.name) +
								'">' + esc(t("iam.breweries.add_member")) + "</button> "
							: "";
						const membersBtn =
							'<button type="button" class="btn btn--small" data-list-members="' +
							b.id +
							'" data-brewery-name="' +
							esc(b.name) +
							'">' + esc(t("iam.breweries.members")) + "</button>";
						const ig = b.instagram
							? ' <a href="' +
								esc(b.instagram) +
								'" target="_blank" rel="noopener noreferrer">' +
								esc(b.instagram) +
								"</a>"
							: "";
						return (
							'<div class="panel__card"><div class="panel__card-head">' +
							logo +
							"<strong>#" +
							b.id +
							" " +
							esc(b.name) +
							"</strong></div>" +
							esc(b.contact_name) +
							" " +
							esc(b.contact_email) +
							" " +
							esc(b.contact_phone || "") +
							ig +
							'<div class="panel__card-actions">' +
							editBtn +
							delBtn +
							addBtn +
							membersBtn +
							"</div></div>"
						);
					})
					.join("") || '<p class="panel__empty">' + esc(t("js.iam.no_breweries")) + "</p>";
			} catch (e) {
				brewEl.textContent = e.message;
			}
		}

		async function fillBreweryAdminSelect(selectEl, selectedID) {
			try {
				const users = await api("/api/users");
				selectEl.innerHTML =
					'<option value="">' + esc(t("common.none")) + "</option>" +
					(users || [])
						.filter((u) => u.active !== false)
						.map(
							(u) =>
								'<option value="' +
								u.id +
								'">' +
								esc(u.username + (u.email ? " (" + u.email + ")" : "")) +
								"</option>"
						)
						.join("");
				if (selectedID) {
					selectEl.value = String(selectedID);
				}
			} catch (e) {
				selectEl.innerHTML = '<option value="">' + esc(t("common.none")) + "</option>";
			}
		}

		async function openBreweryDialog(breweryID) {
			const dlg = panel.querySelector("#iam-brewery-dialog");
			const form = panel.querySelector("#iam-brewery-form");
			const title = panel.querySelector("#iam-brewery-title");
			const adminSel = form.querySelector('[name="brewery_admin_user_id"]');
			const logoBlock = panel.querySelector("#iam-brewery-logo-block");
			const logoErr = panel.querySelector("#iam-brewery-logo-error");
			const logoFile = panel.querySelector("#iam-brewery-logo-file");
			form.reset();
			form.querySelector('[name="brewery_id"]').value = breweryID ? String(breweryID) : "";
			if (logoErr) {
				logoErr.hidden = true;
				logoErr.textContent = "";
			}
			if (logoFile) {
				logoFile.value = "";
			}
			let adminID = "";
			let logoConfigured = false;
			if (breweryID) {
				title.textContent = t("js.iam.edit_brewery");
				const [brewery, members] = await Promise.all([
					api("/api/breweries/" + breweryID),
					api("/api/breweries/" + breweryID + "/members"),
				]);
				form.querySelector('[name="name"]').value = brewery.name || "";
				form.querySelector('[name="contact_name"]').value = brewery.contact_name || "";
				form.querySelector('[name="contact_email"]').value = brewery.contact_email || "";
				form.querySelector('[name="contact_phone"]').value = brewery.contact_phone || "";
				form.querySelector('[name="instagram"]').value = brewery.instagram || "";
				logoConfigured = !!brewery.logo_configured;
				const admin = (members || []).find((m) => m.role === "brewery_admin");
				if (admin) {
					adminID = String(admin.user_id);
				}
				if (logoBlock) {
					logoBlock.hidden = false;
				}
				setBreweryLogoPreview(breweryID, logoConfigured);
			} else {
				title.textContent = t("js.iam.new_brewery");
				if (logoBlock) {
					logoBlock.hidden = true;
				}
				setBreweryLogoPreview(null, false);
			}
			if (isAdmin) {
				await fillBreweryAdminSelect(adminSel, adminID);
			}
			applyRequireRoles(form);
			dlg.showModal();
		}

		const membersDialog = panel.querySelector("#iam-members-dialog");
		const membersClose = panel.querySelector("#iam-members-close");

		async function refreshMembersList(breweryID) {
			const listEl = panel.querySelector("#iam-members-list");
			listEl.textContent = t("js.loading");
			try {
				const members = await api("/api/breweries/" + breweryID + "/members");
				if (!members || !members.length) {
					listEl.innerHTML = '<p class="panel__empty">' + esc(t("js.iam.no_members")) + "</p>";
					return;
				}
				const roles = ["user", "superuser", "brewery_admin"];
				listEl.innerHTML = table(
					[t("common.user"), t("common.role"), ""],
					members
						.map((m) => {
							const opts = roles
								.map(
									(r) =>
										'<option value="' +
										r +
										'"' +
										(m.role === r ? " selected" : "") +
										">" +
										r +
										"</option>"
								)
								.join("");
							return (
								"<tr><td>" +
								esc(m.username) +
								'</td><td><select data-member-role="' +
								m.user_id +
								'" data-prev-role="' +
								esc(m.role) +
								'">' +
								opts +
								'</select></td><td><button type="button" class="btn btn--small" data-remove-member="' +
								m.user_id +
								'">' + esc(t("common.remove")) + "</button></td></tr>"
							);
						})
						.join("")
				);
			} catch (e) {
				listEl.textContent = e.message;
			}
		}

		async function openMembersDialog(breweryID, breweryName) {
			panel.querySelector("#iam-members-brewery-id").value = breweryID;
			panel.querySelector("#iam-members-title").textContent = t("js.iam.members_title", { name: breweryName });
			await refreshMembersList(breweryID);
			membersDialog.showModal();
		}

		if (membersClose) {
			membersClose.addEventListener("click", () => {
				membersDialog.close();
			});
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			const action = el.getAttribute("data-action");
			if (action === "iam-breweries-refresh") {
				refreshBreweries();
			}
			if (action === "iam-brewery-logo-upload") {
				const breweryID = panel.querySelector('#iam-brewery-form [name="brewery_id"]').value;
				const fileInput = panel.querySelector("#iam-brewery-logo-file");
				const errEl = panel.querySelector("#iam-brewery-logo-error");
				errEl.hidden = true;
				const file = fileInput && fileInput.files && fileInput.files[0];
				if (!breweryID) {
					errEl.hidden = false;
					errEl.textContent = t("js.iam.save_before_logo");
					return;
				}
				if (!file) {
					errEl.hidden = false;
					errEl.textContent = t("js.iam.choose_file");
					return;
				}
				try {
					const fd = new FormData();
					fd.append("logo", file);
					const res = await fetch("/api/breweries/" + breweryID + "/logo", {
						method: "PUT",
						headers: { Authorization: "Bearer " + token() },
						body: fd,
					});
					const text = await res.text();
					let data = null;
					try {
						data = text ? JSON.parse(text) : null;
					} catch {
						data = { error: text };
					}
					if (!res.ok) {
						throw new Error((data && data.error) || res.statusText);
					}
					fileInput.value = "";
					setBreweryLogoPreview(breweryID, true);
					refreshBreweries();
				} catch (e) {
					errEl.hidden = false;
					errEl.textContent = e.message;
				}
				return;
			}
			if (action === "iam-brewery-logo-reset") {
				const breweryID = panel.querySelector('#iam-brewery-form [name="brewery_id"]').value;
				const errEl = panel.querySelector("#iam-brewery-logo-error");
				errEl.hidden = true;
				if (!breweryID) {
					return;
				}
				try {
					await api("/api/breweries/" + breweryID + "/logo", { method: "DELETE" });
					setBreweryLogoPreview(breweryID, false);
					refreshBreweries();
				} catch (e) {
					errEl.hidden = false;
					errEl.textContent = e.message;
				}
				return;
			}
			if (action === "iam-breweries-export") {
				try {
					await downloadCSVAuth("/api/breweries/export", "breweries.csv");
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (action === "iam-breweries-import") {
				const input = panel.querySelector("#iam-breweries-import-file");
				if (input) {
					input.value = "";
					input.click();
				}
			}
			if (action === "iam-members-export") {
				try {
					await downloadCSVAuth("/api/breweries/members/export", "members.csv");
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (action === "iam-members-import") {
				const input = panel.querySelector("#iam-members-import-file");
				if (input) {
					input.value = "";
					input.click();
				}
			}
			if (action === "iam-brewery-new") {
				try {
					await openBreweryDialog(null);
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
			if (el.hasAttribute("data-edit-brewery")) {
				try {
					await openBreweryDialog(el.getAttribute("data-edit-brewery"));
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.hasAttribute("data-del-brewery")) {
				const id = el.getAttribute("data-del-brewery");
				const name = el.getAttribute("data-brewery-name") || id;
				const ok = await appConfirm({
					title: t("js.iam.remove_brewery_title"),
					message: t("js.iam.remove_brewery_message", { id: id, name: name }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/breweries/" + id, { method: "DELETE" });
					refreshBreweries();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.hasAttribute("data-add-member")) {
				const breweryID = el.getAttribute("data-add-member");
				const dlg = panel.querySelector("#iam-member-dialog");
				const form = panel.querySelector("#iam-member-form");
				const userSel = form.querySelector('[name="user_id"]');
				form.querySelector('[name="brewery_id"]').value = breweryID;
				userSel.innerHTML = '<option value="">' + esc(t("iam.breweries.select_user")) + "</option>";
				try {
					const [users, members] = await Promise.all([
						api("/api/users"),
						api("/api/breweries/" + breweryID + "/members"),
					]);
					const memberIDs = new Set((members || []).map((m) => m.user_id));
					userSel.innerHTML =
						'<option value="">' + esc(t("iam.breweries.select_user")) + "</option>" +
						(users || [])
							.filter((u) => u.active !== false && !memberIDs.has(u.id))
							.map(
								(u) =>
									'<option value="' +
									u.id +
									'">' +
									esc(u.username + (u.email ? " (" + u.email + ")" : "")) +
									"</option>"
							)
							.join("");
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
					return;
				}
				dlg.showModal();
			}
			if (el.hasAttribute("data-list-members")) {
				const breweryID = el.getAttribute("data-list-members");
				const name = el.getAttribute("data-brewery-name") || "#" + breweryID;
				openMembersDialog(breweryID, name);
			}
			if (el.hasAttribute("data-remove-member")) {
				const breweryID = panel.querySelector("#iam-members-brewery-id").value;
				const userID = el.getAttribute("data-remove-member");
				const ok = await appConfirm({
					title: t("js.iam.remove_member_title"),
					message: t("js.iam.remove_member_message"),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/breweries/" + breweryID + "/members/" + userID, {
						method: "DELETE",
					});
					await refreshMembersList(breweryID);
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
			}
		});

		panel.addEventListener("change", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (!el.hasAttribute("data-member-role")) {
				return;
			}
			const breweryID = panel.querySelector("#iam-members-brewery-id").value;
			const userID = parseInt(el.getAttribute("data-member-role"), 10);
			const prevRole = el.getAttribute("data-prev-role") || "";
			const newRole = el.value;
			if (newRole === prevRole) {
				return;
			}
			const ok = await appConfirm({
				title: t("js.iam.change_role_title"),
				message: t("js.iam.change_role_message", { role: newRole }),
			});
			if (!ok) {
				el.value = prevRole;
				return;
			}
			try {
				await api("/api/breweries/" + breweryID + "/members", {
					method: "POST",
					body: JSON.stringify({ user_id: userID, role: newRole }),
				});
				el.setAttribute("data-prev-role", newRole);
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
				await refreshMembersList(breweryID);
			}
		});

		const breweryDialog = panel.querySelector("#iam-brewery-dialog");
		const breweryForm = panel.querySelector("#iam-brewery-form");
		breweryDialog.addEventListener("close", async () => {
			if (breweryDialog.returnValue !== "save") {
				breweryForm.reset();
				return;
			}
			const fd = new FormData(breweryForm);
			const breweryID = fd.get("brewery_id");
			const adminID = fd.get("brewery_admin_user_id");
			const body = {
				name: fd.get("name"),
				contact_name: fd.get("contact_name"),
				contact_email: fd.get("contact_email"),
				contact_phone: fd.get("contact_phone"),
				instagram: fd.get("instagram"),
			};
			if (isAdmin && adminID) {
				body.brewery_admin_user_id = parseInt(adminID, 10);
			}
			try {
				if (breweryID) {
					await api("/api/breweries/" + breweryID, {
						method: "PUT",
						body: JSON.stringify(body),
					});
				} else {
					await api("/api/breweries", { method: "POST", body: JSON.stringify(body) });
				}
				breweryForm.reset();
				refreshBreweries();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		const memberDialog = panel.querySelector("#iam-member-dialog");
		const memberForm = panel.querySelector("#iam-member-form");
		memberDialog.addEventListener("close", async () => {
			if (memberDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(memberForm);
			const ok = await appConfirm({
				title: t("iam.breweries.add_member"),
				message: t("js.iam.add_member_message"),
			});
			if (!ok) {
				return;
			}
			try {
				await api("/api/breweries/" + fd.get("brewery_id") + "/members", {
					method: "POST",
					body: JSON.stringify({
						user_id: parseInt(fd.get("user_id"), 10),
						role: fd.get("role"),
					}),
				});
				await appInfo({ title: noticeTitle(), message: t("js.iam.member_added") });
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});

		const breweriesImportFile = panel.querySelector("#iam-breweries-import-file");
		if (breweriesImportFile) {
			breweriesImportFile.addEventListener("change", async () => {
				const file = breweriesImportFile.files && breweriesImportFile.files[0];
				if (!file) {
					return;
				}
				try {
					const result = await importCSVAuth("/api/breweries/import", file);
					await appInfo({
						title: t("js.inventory.import_result"),
						message: formatImportResult(result || {}),
					});
					refreshBreweries();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				} finally {
					breweriesImportFile.value = "";
				}
			});
		}

		const membersImportFile = panel.querySelector("#iam-members-import-file");
		if (membersImportFile) {
			membersImportFile.addEventListener("change", async () => {
				const file = membersImportFile.files && membersImportFile.files[0];
				if (!file) {
					return;
				}
				try {
					const result = await importCSVAuth("/api/breweries/members/import", file);
					await appInfo({
						title: t("js.inventory.import_result"),
						message: formatImportResult(result || {}),
					});
					refreshBreweries();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				} finally {
					membersImportFile.value = "";
				}
			});
		}

		refreshBreweries();
	}

	async function loadSettingsBrand(panel) {
		const logoPreview = panel.querySelector("#settings-logo-preview");
		const faviconPreview = panel.querySelector("#settings-favicon-preview");
		const logoErr = panel.querySelector("#settings-logo-error");
		const faviconErr = panel.querySelector("#settings-favicon-error");
		const bgHexInput = panel.querySelector("#settings-logo-bg-hex");
		const bgPicker = panel.querySelector("#settings-logo-bg-picker");
		const bgErr = panel.querySelector("#settings-logo-bg-error");
		const defaultBg = "#6c704a";

		function applyBgLocal(hex) {
			const color = hex || defaultBg;
			if (bgHexInput) {
				bgHexInput.value = color;
			}
			if (bgPicker) {
				bgPicker.value = color;
			}
			if (window.BrewhouseAuth && typeof window.BrewhouseAuth.applyWelcomeLogoBg === "function") {
				window.BrewhouseAuth.applyWelcomeLogoBg(color);
			}
		}

		function bustBrand(kind) {
			const url = "/" + kind + "?t=" + Date.now();
			if (kind === "logo") {
				if (logoPreview) {
					logoPreview.src = url;
				}
				document.querySelectorAll(".splash__logo, .login__logo, .shell__brand-logo, .shell__welcome-logo").forEach((img) => {
					img.src = url;
				});
			} else {
				if (faviconPreview) {
					faviconPreview.src = url;
				}
				const link = document.querySelector('link[rel="icon"]');
				if (link) {
					link.href = url;
				}
			}
		}

		async function uploadBrand(kind, fileInput, errEl) {
			errEl.hidden = true;
			const file = fileInput.files && fileInput.files[0];
			if (!file) {
				errEl.hidden = false;
				errEl.textContent = t("js.iam.choose_file");
				return;
			}
			const fd = new FormData();
			fd.append(kind, file);
			const res = await fetch("/api/settings/" + kind, {
				method: "PUT",
				headers: { Authorization: "Bearer " + token() },
				body: fd,
			});
			const text = await res.text();
			let data = null;
			try {
				data = text ? JSON.parse(text) : null;
			} catch {
				data = { error: text };
			}
			if (!res.ok) {
				throw new Error((data && data.error) || res.statusText);
			}
			fileInput.value = "";
			bustBrand(kind);
		}

		async function resetBrand(kind, errEl) {
			errEl.hidden = true;
			await api("/api/settings/" + kind, { method: "DELETE" });
			bustBrand(kind);
		}

		bustBrand("logo");
		bustBrand("favicon");

		try {
			const cfg = await api("/api/settings/brand-color");
			applyBgLocal((cfg && cfg.logo_bg_hex) || defaultBg);
		} catch (e) {
			applyBgLocal(defaultBg);
		}

		if (bgHexInput && bgPicker) {
			bgHexInput.addEventListener("input", () => {
				const v = bgHexInput.value.trim();
				if (/^#[0-9A-Fa-f]{6}$/.test(v)) {
					bgPicker.value = v.toLowerCase();
				}
			});
			bgPicker.addEventListener("input", () => {
				bgHexInput.value = bgPicker.value;
			});
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			const action = el.getAttribute("data-action");
			if (action === "settings-logo-upload") {
				try {
					await uploadBrand("logo", panel.querySelector("#settings-logo-file"), logoErr);
				} catch (e) {
					logoErr.hidden = false;
					logoErr.textContent = e.message;
				}
			}
			if (action === "settings-logo-reset") {
				try {
					await resetBrand("logo", logoErr);
				} catch (e) {
					logoErr.hidden = false;
					logoErr.textContent = e.message;
				}
			}
			if (action === "settings-favicon-upload") {
				try {
					await uploadBrand(
						"favicon",
						panel.querySelector("#settings-favicon-file"),
						faviconErr
					);
				} catch (e) {
					faviconErr.hidden = false;
					faviconErr.textContent = e.message;
				}
			}
			if (action === "settings-favicon-reset") {
				try {
					await resetBrand("favicon", faviconErr);
				} catch (e) {
					faviconErr.hidden = false;
					faviconErr.textContent = e.message;
				}
			}
			if (action === "settings-logo-bg-save") {
				bgErr.hidden = true;
				try {
					const hex = (bgHexInput && bgHexInput.value.trim()) || "";
					const cfg = await api("/api/settings/brand-color", {
						method: "PUT",
						body: JSON.stringify({ logo_bg_hex: hex }),
					});
					applyBgLocal((cfg && cfg.logo_bg_hex) || defaultBg);
				} catch (e) {
					bgErr.hidden = false;
					bgErr.textContent = e.message;
				}
			}
			if (action === "settings-logo-bg-reset") {
				bgErr.hidden = true;
				try {
					const cfg = await api("/api/settings/brand-color", { method: "DELETE" });
					applyBgLocal((cfg && cfg.logo_bg_hex) || defaultBg);
				} catch (e) {
					bgErr.hidden = false;
					bgErr.textContent = e.message;
				}
			}
		});
	}

	async function loadSettingsTanks(panel) {
		const tanksEl = panel.querySelector("#settings-tanks");

		async function refresh() {
			try {
				const tanks = await api("/api/settings/tanks");
				tanksEl.innerHTML = table(
					[t("common.id"), t("common.name"), t("js.settings.capacity_l"), t("common.active"), ""],
					(tanks || [])
						.map((tank) => {
							const toggleLabel = tank.active ? t("js.settings.disable") : t("js.settings.enable");
							return (
								"<tr><td>" +
								tank.id +
								"</td><td>" +
								esc(tank.name) +
								"</td><td>" +
								tank.capacity_liters +
								"</td><td>" +
								(tank.active ? t("js.settings.yes") : t("js.settings.no")) +
								'</td><td><button type="button" class="btn btn--small" data-tank-edit="' +
								tank.id +
								'" data-name="' +
								esc(tank.name) +
								'" data-capacity="' +
								tank.capacity_liters +
								'">' +
								esc(t("common.edit")) +
								'</button> <button type="button" class="btn btn--small" data-tank-active="' +
								tank.id +
								'" data-active="' +
								(tank.active ? "0" : "1") +
								'">' +
								toggleLabel +
								'</button> <button type="button" class="btn btn--small" data-tank-delete="' +
								tank.id +
								'">' +
								esc(t("common.remove")) +
								"</button></td></tr>"
							);
						})
						.join("")
				);
			} catch (e) {
				tanksEl.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.hasAttribute("data-tank-edit")) {
				const id = el.getAttribute("data-tank-edit");
				const values = await appPrompt({
					title: t("js.settings.edit_tank"),
					fields: [
						{
							name: "name",
							label: t("js.settings.tank_name"),
							type: "text",
							value: el.getAttribute("data-name") || "",
						},
						{
							name: "capacity_liters",
							label: t("js.settings.capacity_liters"),
							type: "number",
							step: "any",
							value: el.getAttribute("data-capacity") || "0",
						},
					],
				});
				if (!values || !String(values.name || "").trim()) {
					return;
				}
				try {
					await api("/api/settings/tanks/" + id, {
						method: "PUT",
						body: JSON.stringify({
							name: String(values.name).trim(),
							capacity_liters: parseFloat(values.capacity_liters),
						}),
					});
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.hasAttribute("data-tank-active")) {
				try {
					await api("/api/settings/tanks/" + el.getAttribute("data-tank-active") + "/active", {
						method: "POST",
						body: JSON.stringify({ active: el.getAttribute("data-active") === "1" }),
					});
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.hasAttribute("data-tank-delete")) {
				const id = el.getAttribute("data-tank-delete");
				const ok = await appConfirm({
					title: t("js.settings.remove_tank_title"),
					message: t("js.settings.remove_tank_message", { id: id }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/settings/tanks/" + id, { method: "DELETE" });
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.getAttribute("data-action") !== "settings-tank-new") {
				return;
			}
			const values = await appPrompt({
				title: t("js.settings.new_tank"),
				fields: [
					{ name: "name", label: t("js.settings.tank_name"), type: "text" },
					{ name: "capacity_liters", label: t("js.settings.capacity_liters"), type: "number", step: "any", value: "1000" },
				],
			});
			if (!values || !String(values.name || "").trim()) {
				return;
			}
			try {
				await api("/api/settings/tanks", {
					method: "POST",
					body: JSON.stringify({
						name: String(values.name).trim(),
						capacity_liters: parseFloat(values.capacity_liters),
					}),
				});
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});
		refresh();
	}

	async function loadSettingsMultipliers(panel) {
		const multsEl = panel.querySelector("#settings-mults");

		async function refresh() {
			try {
				const mults = await api("/api/settings/multipliers");
				multsEl.innerHTML = table(
					[t("common.id"), t("common.name"), "×", t("common.active"), ""],
					(mults || [])
						.map((m) => {
							const toggleLabel = m.active ? t("js.settings.disable") : t("js.settings.enable");
							return (
								"<tr><td>" +
								m.id +
								"</td><td>" +
								esc(m.name) +
								"</td><td>" +
								m.multiplier +
								"</td><td>" +
								(m.active ? t("js.settings.yes") : t("js.settings.no")) +
								'</td><td><button type="button" class="btn btn--small" data-mult-edit="' +
								m.id +
								'" data-name="' +
								esc(m.name) +
								'" data-multiplier="' +
								m.multiplier +
								'">' + esc(t("common.edit")) + '</button> <button type="button" class="btn btn--small" data-mult-active="' +
								m.id +
								'" data-active="' +
								(m.active ? "0" : "1") +
								'">' +
								toggleLabel +
								'</button> <button type="button" class="btn btn--small" data-mult-delete="' +
								m.id +
								'">' + esc(t("common.remove")) + "</button></td></tr>"
							);
						})
						.join("")
				);
			} catch (e) {
				multsEl.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.hasAttribute("data-mult-edit")) {
				const id = el.getAttribute("data-mult-edit");
				const values = await appPrompt({
					title: t("js.settings.edit_multiplier"),
					fields: [
						{
							name: "name",
							label: t("js.settings.multiplier_name"),
							type: "text",
							value: el.getAttribute("data-name") || "",
						},
						{
							name: "multiplier",
							label: t("delivery.multiplier"),
							type: "number",
							step: "any",
							value: el.getAttribute("data-multiplier") || "1",
						},
					],
				});
				if (!values || !String(values.name || "").trim()) {
					return;
				}
				try {
					await api("/api/settings/multipliers/" + id, {
						method: "PUT",
						body: JSON.stringify({
							name: String(values.name).trim(),
							multiplier: parseFloat(values.multiplier),
						}),
					});
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.hasAttribute("data-mult-active")) {
				try {
					await api(
						"/api/settings/multipliers/" + el.getAttribute("data-mult-active") + "/active",
						{
							method: "POST",
							body: JSON.stringify({ active: el.getAttribute("data-active") === "1" }),
						}
					);
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.hasAttribute("data-mult-delete")) {
				const id = el.getAttribute("data-mult-delete");
				const ok = await appConfirm({
					title: t("js.settings.remove_multiplier_title"),
					message: t("js.settings.remove_multiplier_message", { id: id }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/settings/multipliers/" + id, { method: "DELETE" });
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.getAttribute("data-action") !== "settings-mult-new") {
				return;
			}
			const values = await appPrompt({
				title: t("js.settings.new_multiplier"),
				fields: [
					{ name: "name", label: t("js.settings.multiplier_name"), type: "text" },
					{ name: "multiplier", label: t("delivery.multiplier"), type: "number", step: "any", value: "1.5" },
				],
			});
			if (!values || !String(values.name || "").trim()) {
				return;
			}
			try {
				await api("/api/settings/multipliers", {
					method: "POST",
					body: JSON.stringify({
						name: String(values.name).trim(),
						multiplier: parseFloat(values.multiplier),
					}),
				});
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});
		refresh();
	}

	async function loadSettingsSuppliers(panel) {
		const listEl = panel.querySelector("#settings-suppliers");

		async function refresh() {
			try {
				const suppliers = await api("/api/settings/suppliers");
				listEl.innerHTML = table(
					[
						t("common.id"),
						t("common.name"),
						t("settings.suppliers.adjust_percent"),
						t("common.active"),
						"",
					],
					(suppliers || [])
						.map((s) => {
							const toggleLabel = s.active ? t("js.settings.disable") : t("js.settings.enable");
							return (
								"<tr><td>" +
								s.id +
								"</td><td>" +
								esc(s.name) +
								"</td><td>" +
								s.adjust_percent +
								"</td><td>" +
								(s.active ? t("js.settings.yes") : t("js.settings.no")) +
								'</td><td><button type="button" class="btn btn--small" data-supplier-edit="' +
								s.id +
								'" data-name="' +
								esc(s.name) +
								'" data-adjust="' +
								s.adjust_percent +
								'">' +
								esc(t("common.edit")) +
								'</button> <button type="button" class="btn btn--small" data-supplier-active="' +
								s.id +
								'" data-active="' +
								(s.active ? "0" : "1") +
								'">' +
								toggleLabel +
								'</button> <button type="button" class="btn btn--small" data-supplier-delete="' +
								s.id +
								'">' +
								esc(t("common.remove")) +
								"</button></td></tr>"
							);
						})
						.join("")
				);
			} catch (e) {
				listEl.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.hasAttribute("data-supplier-edit")) {
				const id = el.getAttribute("data-supplier-edit");
				const values = await appPrompt({
					title: t("js.settings.edit_supplier"),
					fields: [
						{
							name: "name",
							label: t("js.settings.supplier_name"),
							type: "text",
							value: el.getAttribute("data-name") || "",
						},
						{
							name: "adjust_percent",
							label: t("js.settings.adjust_percent"),
							type: "number",
							step: "any",
							value: el.getAttribute("data-adjust") || "0",
						},
					],
				});
				if (!values || !String(values.name || "").trim()) {
					return;
				}
				try {
					await api("/api/settings/suppliers/" + id, {
						method: "PUT",
						body: JSON.stringify({
							name: String(values.name).trim(),
							adjust_percent: parseFloat(values.adjust_percent) || 0,
						}),
					});
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.hasAttribute("data-supplier-active")) {
				try {
					await api(
						"/api/settings/suppliers/" + el.getAttribute("data-supplier-active") + "/active",
						{
							method: "POST",
							body: JSON.stringify({ active: el.getAttribute("data-active") === "1" }),
						}
					);
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.hasAttribute("data-supplier-delete")) {
				const id = el.getAttribute("data-supplier-delete");
				const ok = await appConfirm({
					title: t("js.settings.remove_supplier_title"),
					message: t("js.settings.remove_supplier_message", { id: id }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/settings/suppliers/" + id, { method: "DELETE" });
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.getAttribute("data-action") !== "settings-supplier-new") {
				return;
			}
			const values = await appPrompt({
				title: t("js.settings.new_supplier"),
				fields: [
					{ name: "name", label: t("js.settings.supplier_name"), type: "text" },
					{
						name: "adjust_percent",
						label: t("js.settings.adjust_percent"),
						type: "number",
						step: "any",
						value: "0",
					},
				],
			});
			if (!values || !String(values.name || "").trim()) {
				return;
			}
			try {
				await api("/api/settings/suppliers", {
					method: "POST",
					body: JSON.stringify({
						name: String(values.name).trim(),
						adjust_percent: parseFloat(values.adjust_percent) || 0,
					}),
				});
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});
		refresh();
	}

	async function loadSettingsBeerPrice(panel) {
		const beerForm = panel.querySelector("#settings-beer-price-form");
		const beerErr = panel.querySelector("#settings-beer-price-error");

		async function refresh() {
			try {
				const beer = await api("/api/settings/beer-price");
				beerForm.querySelector('[name="min_net_sek_per_liter"]').value =
					beer.min_net_sek_per_liter ?? 0;
			} catch (e) {
				beerErr.hidden = false;
				beerErr.textContent = e.message;
			}
		}

		beerForm.addEventListener("submit", async (ev) => {
			ev.preventDefault();
			beerErr.hidden = true;
			const fd = new FormData(beerForm);
			try {
				await api("/api/settings/beer-price", {
					method: "PUT",
					body: JSON.stringify({
						min_net_sek_per_liter: parseFloat(fd.get("min_net_sek_per_liter")),
					}),
				});
			} catch (e) {
				beerErr.hidden = false;
				beerErr.textContent = e.message;
			}
		});
		refresh();
	}

	async function loadSettingsRegional(panel) {
		const form = panel.querySelector("#settings-regional-form");
		const errEl = panel.querySelector("#settings-regional-error");
		const currencySel = panel.querySelector("#settings-regional-currency");
		const languageSel = panel.querySelector("#settings-regional-language");

		async function refresh() {
			try {
				const cfg = await api("/api/settings/regional");
				currencySel.value = cfg.currency_code || "SEK";
				languageSel.value = cfg.language || "en";
			} catch (e) {
				errEl.hidden = false;
				errEl.textContent = e.message;
			}
		}

		form.addEventListener("submit", async (ev) => {
			ev.preventDefault();
			errEl.hidden = true;
			try {
				const cfg = await api("/api/settings/regional", {
					method: "PUT",
					body: JSON.stringify({
						currency_code: currencySel.value,
						language: languageSel.value,
					}),
				});
				if (window.BH_I18N && typeof window.BH_I18N.applyRegional === "function") {
					await window.BH_I18N.applyRegional(cfg);
				}
			} catch (e) {
				errEl.hidden = false;
				errEl.textContent = e.message;
			}
		});
		refresh();
	}

	async function loadSettingsHygiene(panel) {
		const hygEl = panel.querySelector("#settings-hygiene");

		async function refresh() {
			try {
				const hyg = await api("/api/settings/hygiene-routines");
				hygEl.innerHTML = table(
					[t("common.name"), t("js.settings.description"), t("js.settings.sort"), ""],
					(hyg || [])
						.map((r) => {
							return (
								"<tr><td>" +
								esc(r.name) +
								"</td><td>" +
								esc(r.description || "") +
								"</td><td>" +
								r.sort_order +
								'</td><td><button type="button" class="btn btn--small" data-hygiene-edit="' +
								r.id +
								'" data-name="' +
								esc(r.name) +
								'" data-description="' +
								esc(r.description || "") +
								'" data-sort="' +
								r.sort_order +
								'">' + esc(t("common.edit")) + '</button> <button type="button" class="btn btn--small" data-hygiene-delete="' +
								r.id +
								'">' + esc(t("common.remove")) + "</button></td></tr>"
							);
						})
						.join("")
				);
			} catch (e) {
				hygEl.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			if (el.hasAttribute("data-hygiene-edit")) {
				const id = el.getAttribute("data-hygiene-edit");
				const values = await appPrompt({
					title: t("js.settings.edit_hygiene"),
					fields: [
						{
							name: "name",
							label: t("common.name"),
							type: "text",
							value: el.getAttribute("data-name") || "",
						},
						{
							name: "description",
							label: t("js.settings.description"),
							type: "text",
							value: el.getAttribute("data-description") || "",
						},
						{
							name: "sort_order",
							label: t("js.settings.sort_order"),
							type: "number",
							step: "1",
							value: el.getAttribute("data-sort") || "0",
						},
					],
				});
				if (!values || !String(values.name || "").trim()) {
					return;
				}
				try {
					await api("/api/settings/hygiene-routines/" + id, {
						method: "PUT",
						body: JSON.stringify({
							name: String(values.name).trim(),
							description: String(values.description || "").trim(),
							sort_order: parseInt(values.sort_order, 10) || 0,
						}),
					});
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.hasAttribute("data-hygiene-delete")) {
				const id = el.getAttribute("data-hygiene-delete");
				const ok = await appConfirm({
					title: t("js.settings.remove_hygiene_title"),
					message: t("js.settings.remove_hygiene_message", { id: id }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/settings/hygiene-routines/" + id, { method: "DELETE" });
					refresh();
				} catch (e) {
					await appInfo({ title: noticeTitle(), message: e.message });
				}
				return;
			}
			if (el.getAttribute("data-action") === "settings-hygiene-refresh") {
				refresh();
				return;
			}
			if (el.getAttribute("data-action") !== "settings-hygiene-new") {
				return;
			}
			const values = await appPrompt({
				title: t("js.settings.new_hygiene"),
				fields: [
					{ name: "name", label: t("common.name"), type: "text" },
					{ name: "description", label: t("js.settings.description"), type: "text" },
					{ name: "sort_order", label: t("js.settings.sort_order"), type: "number", step: "1", value: "0" },
				],
			});
			if (!values || !String(values.name || "").trim()) {
				return;
			}
			try {
				await api("/api/settings/hygiene-routines", {
					method: "POST",
					body: JSON.stringify({
						name: String(values.name).trim(),
						description: String(values.description || "").trim(),
						sort_order: parseInt(values.sort_order, 10) || 0,
					}),
				});
				refresh();
			} catch (e) {
				await appInfo({ title: noticeTitle(), message: e.message });
			}
		});
		refresh();
	}

	async function loadSettingsBackup(panel) {
		const listEl = panel.querySelector("#settings-backup");
		const errEl = panel.querySelector("#settings-backup-error");
		const uploadInput = panel.querySelector("#settings-backup-upload");

		function formatBytes(n) {
			if (n < 1024) {
				return n + " B";
			}
			if (n < 1024 * 1024) {
				return (n / 1024).toFixed(1) + " KB";
			}
			return (n / (1024 * 1024)).toFixed(1) + " MB";
		}

		function formatWhen(iso) {
			if (!iso) {
				return "";
			}
			const d = new Date(iso);
			if (Number.isNaN(d.getTime())) {
				return iso;
			}
			return d.toLocaleString();
		}

		function kindLabel(kind) {
			if (kind === "daily") {
				return t("js.settings.backup_daily");
			}
			if (kind === "monthly") {
				return t("js.settings.backup_monthly");
			}
			if (kind === "manual") {
				return t("js.settings.backup_manual");
			}
			return kind || "";
		}

		function showErr(msg) {
			if (!errEl) {
				return;
			}
			if (!msg) {
				errEl.hidden = true;
				errEl.textContent = "";
				return;
			}
			errEl.hidden = false;
			errEl.textContent = msg;
		}

		async function refresh() {
			showErr("");
			try {
				const list = await api("/api/settings/backup");
				if (!list || !list.length) {
					listEl.innerHTML = '<p class="panel__empty">' + esc(t("js.settings.no_backups")) + "</p>";
					return;
				}
				listEl.innerHTML = table(
					[t("js.settings.backup_taken"), t("js.settings.backup_type"), t("js.settings.backup_size"), t("js.settings.backup_file"), ""],
					list
						.map((b) => {
							const name = esc(b.name || "");
							return (
								"<tr><td>" +
								esc(formatWhen(b.created_at)) +
								"</td><td>" +
								esc(kindLabel(b.kind)) +
								"</td><td>" +
								esc(formatBytes(b.size_bytes || 0)) +
								"</td><td>" +
								name +
								'</td><td class="panel__row-actions">' +
								'<button type="button" class="btn btn--small" data-action="settings-backup-download" data-name="' +
								name +
								'">' +
								esc(t("js.settings.download")) +
								"</button> " +
								'<button type="button" class="btn btn--small" data-action="settings-backup-restore" data-name="' +
								name +
								'">' +
								esc(t("js.settings.restore")) +
								"</button> " +
								'<button type="button" class="btn btn--small" data-action="settings-backup-delete" data-name="' +
								name +
								'">' +
								esc(t("common.delete")) +
								"</button></td></tr>"
							);
						})
						.join("")
				);
			} catch (e) {
				listEl.textContent = e.message;
			}
		}

		async function downloadBackup(name) {
			await downloadCSVAuth("/api/settings/backup/" + encodeURIComponent(name) + "/download", name);
		}

		async function restoreNamed(name) {
			const ok = await appConfirm({
				title: t("js.settings.restore_backup_title"),
				message: t("js.settings.restore_backup_message", { name: name }),
			});
			if (!ok) {
				return;
			}
			await api("/api/settings/backup/" + encodeURIComponent(name) + "/restore", {
				method: "POST",
			});
			await appInfo({
				title: t("js.settings.restore_complete_title"),
				message: t("js.settings.restore_complete_message"),
			});
			window.location.reload();
		}

		async function restoreUpload(file) {
			const ok = await appConfirm({
				title: t("js.settings.restore_from_file"),
				message:
					t("js.settings.restore_upload_message"),
			});
			if (!ok) {
				return;
			}
			const fd = new FormData();
			fd.append("file", file);
			const res = await fetch("/api/settings/backup/restore-upload", {
				method: "POST",
				headers: { Authorization: "Bearer " + token() },
				body: fd,
			});
			const text = await res.text();
			let data = null;
			try {
				data = text ? JSON.parse(text) : null;
			} catch {
				data = { error: text };
			}
			if (!res.ok) {
				throw new Error((data && data.error) || res.statusText);
			}
			await appInfo({
				title: t("js.settings.restore_complete_title"),
				message: t("js.settings.restore_complete_message"),
			});
			window.location.reload();
		}

		panel.addEventListener("click", async (ev) => {
			const el = ev.target;
			if (!(el instanceof HTMLElement)) {
				return;
			}
			const action = el.getAttribute("data-action");
			if (action === "settings-backup-refresh") {
				refresh();
				return;
			}
			if (action === "settings-backup-now") {
				showErr("");
				try {
					await api("/api/settings/backup", { method: "POST" });
					refresh();
				} catch (e) {
					showErr(e.message);
				}
				return;
			}
			const name = el.getAttribute("data-name");
			if (action === "settings-backup-download" && name) {
				try {
					await downloadBackup(name);
				} catch (e) {
					showErr(e.message);
				}
				return;
			}
			if (action === "settings-backup-restore" && name) {
				try {
					await restoreNamed(name);
				} catch (e) {
					showErr(e.message);
				}
				return;
			}
			if (action === "settings-backup-delete" && name) {
				const ok = await appConfirm({
					title: t("js.settings.delete_backup_title"),
					message: t("js.settings.delete_backup_message", { name: name }),
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/settings/backup/" + encodeURIComponent(name), {
						method: "DELETE",
					});
					refresh();
				} catch (e) {
					showErr(e.message);
				}
			}
		});

		if (uploadInput) {
			uploadInput.addEventListener("change", async () => {
				const file = uploadInput.files && uploadInput.files[0];
				uploadInput.value = "";
				if (!file) {
					return;
				}
				showErr("");
				try {
					await restoreUpload(file);
				} catch (e) {
					showErr(e.message);
				}
			});
		}

		refresh();
	}
})();

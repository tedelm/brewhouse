(() => {
	const TOKEN_KEY = "brewhouse_token";

	function token() {
		return sessionStorage.getItem(TOKEN_KEY) || "";
	}

	async function api(path, options = {}) {
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
		if (!res.ok) {
			const err = new Error((data && data.error) || res.statusText);
			err.status = res.status;
			err.data = data;
			throw err;
		}
		return data;
	}

	function esc(s) {
		return String(s ?? "")
			.replace(/&/g, "&amp;")
			.replace(/</g, "&lt;")
			.replace(/>/g, "&gt;")
			.replace(/"/g, "&quot;");
	}

	function table(headers, rowsHtml) {
		return (
			'<div class="table-scroll"><table class="data-table"><thead><tr>' +
			headers.map((h) => "<th>" + esc(h) + "</th>").join("") +
			"</tr></thead><tbody>" +
			rowsHtml +
			"</tbody></table></div>"
		);
	}

	function fmtMoney(n) {
		if (n == null || n === "" || Number.isNaN(Number(n))) {
			return "";
		}
		return Number(n).toFixed(2);
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
		titleEl.textContent = (opts && opts.title) || "Confirm";
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
		titleEl.textContent = (opts && opts.title) || "Info";
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

	function appPrompt(opts) {
		const dialog = document.getElementById("app-prompt-dialog");
		const form = document.getElementById("app-prompt-form");
		const titleEl = document.getElementById("app-prompt-title");
		const fieldsEl = document.getElementById("app-prompt-fields");
		if (!dialog || !form || !titleEl || !fieldsEl) {
			return Promise.resolve(null);
		}
		const fields = (opts && opts.fields) || [];
		titleEl.textContent = (opts && opts.title) || "Input";
		fieldsEl.innerHTML = fields
			.map((f) => {
				const type = f.type || "text";
				const step = f.step != null ? ' step="' + esc(String(f.step)) + '"' : "";
				const min = f.min != null ? ' min="' + esc(String(f.min)) + '"' : "";
				const required = f.required === false ? "" : " required";
				const value = f.value != null ? esc(String(f.value)) : "";
				return (
					"<label>" +
					esc(f.label || f.name) +
					' <input name="' +
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
	}

	function showForbidden(panel) {
		panel.innerHTML = '<p class="panel__empty">Forbidden</p>';
	}

	document.body.addEventListener("htmx:configRequest", (event) => {
		const t = token();
		if (t) {
			event.detail.headers.Authorization = "Bearer " + t;
		}
	});

	document.body.addEventListener("htmx:afterSwap", (event) => {
		if (event.detail.target && event.detail.target.id === "main-content") {
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
		if ((kind === "iam" || (kind && kind.startsWith("iam-")) || (kind && kind.startsWith("settings"))) && !canRoles("admin")) {
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
			economy: loadEconomy,
			"economy-deliveries": loadEconomyDeliveries,
			delivery: loadDelivery,
			"iam-users": loadIAMUsers,
			"iam-breweries": loadIAMBreweries,
			"settings-brand": loadSettingsBrand,
			"settings-tanks": loadSettingsTanks,
			"settings-multipliers": loadSettingsMultipliers,
			"settings-beer-price": loadSettingsBeerPrice,
			"settings-hygiene": loadSettingsHygiene,
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
			panel.querySelector("#recipe-ingredients").innerHTML = "";
			if (recipe && recipe.id) {
				form.elements.namedItem("id").value = String(recipe.id);
				brewerySel.value = String(recipe.brewery_id);
				brewerySel.disabled = true;
				form.elements.namedItem("name").value = recipe.name || "";
				if (titleEl) {
					titleEl.textContent = "Edit recipe";
				}
				if (saveBtn) {
					saveBtn.textContent = "Save";
				}
				const ings = recipe.ingredients || [];
				if (ings.length) {
					ings.forEach((ing) => addIngRow(panel, ing.inventory_item_id, ing.qty, ing.unit));
				} else {
					addIngRow(panel);
				}
			} else {
				form.elements.namedItem("id").value = "";
				brewerySel.disabled = false;
				form.elements.namedItem("name").value = "";
				if (titleEl) {
					titleEl.textContent = "New recipe";
				}
				if (saveBtn) {
					saveBtn.textContent = "Create";
				}
				addIngRow(panel);
			}
			dialog.showModal();
		}

		async function refresh() {
			list.textContent = "Loading…";
			try {
				const qs = showHidden() ? "?include_hidden=1" : "";
				const recipes = await api("/api/recipes" + qs);
				if (!recipes || !recipes.length) {
					list.innerHTML = "<p class=\"panel__empty\">No recipes yet.</p>";
					return;
				}
				list.innerHTML = table(
					["ID", "Name", "Brewery", "Status", "Actions"],
					recipes
						.map((r) => {
							let actions = "";
							if (editableStatus(r.status)) {
								actions +=
									'<button type="button" class="btn btn--small" data-edit-recipe="' +
									r.id +
									'">Edit</button> ';
								actions +=
									'<button type="button" class="btn btn--small" data-del-recipe="' +
									r.id +
									'">Delete</button>';
							}
							if (r.status === "delivered" && r.active !== false) {
								actions +=
									' <button type="button" class="btn btn--small" data-hide-recipe="' +
									r.id +
									'">Hide</button>';
							}
							if (r.status === "delivered" && r.active === false) {
								actions +=
									' <button type="button" class="btn btn--small" data-unhide-recipe="' +
									r.id +
									'">Unhide</button>';
							}
							if (r.status === "ready_for_delivery") {
								actions +=
									' <button type="button" class="btn btn--small" data-deliver="' +
									r.id +
									'">Deliver</button>';
							}
							const statusLabel =
								r.status === "delivered" && r.active === false
									? "delivered (hidden)"
									: r.status;
							return (
								"<tr><td>" +
								r.id +
								"</td><td>" +
								esc(r.name) +
								"</td><td>" +
								esc(r.brewery_name || String(r.brewery_id)) +
								"</td><td>" +
								esc(statusLabel) +
								"</td><td>" +
								actions +
								"</td></tr>"
							);
						})
						.join("")
				);
			} catch (e) {
				list.textContent = e.message;
			}
		}

		function showShortfallNotice(lines, orderId) {
			const lead = panel.querySelector("#recipe-shortfall-lead");
			if (lead) {
				lead.textContent = orderId
					? "Available stock was checked out. Missing amounts were added to planning order #" +
					  orderId +
					  "."
					: "Available stock was checked out. Missing amounts could not be ordered automatically.";
			}
			shortfallNotice.hidden = true;
			shortfallError.hidden = true;
			shortfallList.innerHTML = (lines || [])
				.map(
					(s) =>
						"<li>" +
						esc(s.name) +
						" — missing " +
						esc(String(s.missing)) +
						"</li>"
				)
				.join("");
			shortfallDialog.showModal();
		}

		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			if (t.getAttribute("data-action") === "recipe-refresh") {
				refresh();
			}
			if (t.getAttribute("data-action") === "recipe-new") {
				try {
					await openRecipeDialog(null);
				} catch (e) {
					alert(e.message);
				}
			}
			if (t.getAttribute("data-action") === "recipe-add-ing") {
				addIngRow(panel);
			}
			if (t.getAttribute("data-action") === "recipe-remove-ing") {
				const row = t.closest(".ing-row");
				if (row) {
					row.remove();
				}
			}
			if (t.hasAttribute("data-edit-recipe")) {
				const id = t.getAttribute("data-edit-recipe");
				try {
					const recipe = await api("/api/recipes/" + id);
					await openRecipeDialog(recipe);
				} catch (e) {
					alert(e.message);
				}
			}
			if (t.hasAttribute("data-del-recipe")) {
				const id = t.getAttribute("data-del-recipe");
				const ok = await appConfirm({
					title: "Delete recipe",
					message: "Delete recipe #" + id + " and restore checked-out inventory?",
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/recipes/" + id, { method: "DELETE" });
					refresh();
				} catch (e) {
					alert(e.message);
				}
			}
			if (t.hasAttribute("data-hide-recipe")) {
				const id = t.getAttribute("data-hide-recipe");
				try {
					await api("/api/recipes/" + id + "/active", {
						method: "POST",
						body: JSON.stringify({ active: false }),
					});
					refresh();
				} catch (e) {
					alert(e.message);
				}
			}
			if (t.hasAttribute("data-unhide-recipe")) {
				const id = t.getAttribute("data-unhide-recipe");
				try {
					await api("/api/recipes/" + id + "/active", {
						method: "POST",
						body: JSON.stringify({ active: true }),
					});
					refresh();
				} catch (e) {
					alert(e.message);
				}
			}
			if (t.hasAttribute("data-deliver")) {
				const id = t.getAttribute("data-deliver");
				try {
					await api("/api/recipes/" + id + "/deliver", { method: "POST" });
					refresh();
				} catch (e) {
					alert(e.message);
				}
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
					showShortfallNotice(result.shortfalls, result.order_id);
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

	function unitOptions(selected) {
		const units = ["kg", "g", "L", "ml"];
		const sel = selected || "kg";
		return units
			.map((u) => '<option value="' + u + '"' + (u === sel ? " selected" : "") + ">" + u + "</option>")
			.join("");
	}

	function addIngRow(panel, selectedId, qty, unit) {
		const items = panel.querySelector("#recipe-form")._items || [];
		const wrap = panel.querySelector("#recipe-ingredients");
		const div = document.createElement("div");
		div.className = "ing-row";
		const first = items[0];
		const initialId = selectedId || (first && first.id);
		const initialItem = items.find((i) => i.id === initialId) || first;
		const initialUnit = unit || (initialItem && initialItem.unit) || "kg";
		div.innerHTML =
			'<label>Item <select name="item_id">' +
			items
				.map(
					(i) =>
						'<option value="' +
						i.id +
						'" data-unit="' +
						esc(i.unit || "kg") +
						'" data-stock="' +
						i.qty +
						'"' +
						(initialId && initialId === i.id ? " selected" : "") +
						">" +
						esc(i.category + " / " + i.name + " (stock " + i.qty + " " + (i.unit || "") + ")") +
						"</option>"
				)
				.join("") +
			'</select></label>' +
			'<label>Qty <input name="qty" type="number" step="any" value="' +
			(qty != null ? qty : 1) +
			'" min="0"></label>' +
			"<label>Unit <select name=\"unit\">" +
			unitOptions(initialUnit) +
			"</select></label>" +
			'<span class="ing-row__stock"></span>' +
			'<button type="button" class="btn btn--small" data-action="recipe-remove-ing">Remove</button>';
		wrap.appendChild(div);
		const sel = div.querySelector('[name="item_id"]');
		const unitSel = div.querySelector('[name="unit"]');
		const stockEl = div.querySelector(".ing-row__stock");
		function syncStock() {
			const opt = sel.options[sel.selectedIndex];
			if (!opt) {
				return;
			}
			const stock = opt.getAttribute("data-stock");
			const u = opt.getAttribute("data-unit") || "";
			stockEl.textContent = "In stock: " + stock + " " + u;
		}
		sel.addEventListener("change", () => {
			const opt = sel.options[sel.selectedIndex];
			if (opt) {
				unitSel.value = opt.getAttribute("data-unit") || "kg";
			}
			syncStock();
		});
		syncStock();
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
			selectEl.innerHTML = '<option value="">' + esc(emptyLabel || "No recipes") + "</option>";
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
			list.textContent = "Loading…";
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
				fillRecipeSelect(recipeSel, bookable, "No bookable recipes");
				if (!bookings || !bookings.length) {
					list.innerHTML = "<p class=\"panel__empty\">No bookings in range.</p>";
					return;
				}
				list.innerHTML = table(
					["Date", "End date", "Brewery", "Recipe", "Tank", "Actions"],
					bookings
						.map((b) => {
							let actions = "";
							if (b.status === "scheduled" && b.recipe_id) {
								actions =
									'<button type="button" class="btn btn--small" data-action="schedule-unbook" data-recipe-id="' +
									b.recipe_id +
									'">Remove</button>';
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
			} catch (e) {
				list.textContent = e.message;
			}
		}

		panel.querySelector('[data-action="schedule-refresh"]').addEventListener("click", refresh);
		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			if (t.getAttribute("data-action") !== "schedule-unbook") {
				return;
			}
			const recipeId = t.getAttribute("data-recipe-id");
			if (!recipeId) {
				return;
			}
			const ok = await appConfirm({
				title: "Remove from schedule",
				message: "Remove this recipe from the schedule? The batch stays; bookings are cleared.",
			});
			if (!ok) {
				return;
			}
			try {
				await api("/api/recipes/" + recipeId + "/schedule", { method: "DELETE" });
				refresh();
			} catch (e) {
				alert(e.message);
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
			list.textContent = "Loading…";
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
				fillRecipeSelect(recipeSel, selectable, "No batches ready");
				if (prev && byID[prev]) {
					recipeSel.value = prev;
				}
				fillBrewdayFields(byID[recipeSel.value]);
				if (!selectable.length) {
					list.innerHTML = "<p class=\"panel__empty\">No scheduled, brewday, or hygiene-done batches.</p>";
					return;
				}
				list.innerHTML = table(
					["ID", "Name", "Brewery", "Status", "OG", "Brew vol (L)", ""],
					selectable
						.map((r) => {
							let actions = "";
							if (r.status === "brewday") {
								actions =
									'<button type="button" class="btn btn--small" data-action="brewday-edit" data-id="' +
									r.id +
									'">Edit</button> ' +
									'<button type="button" class="btn btn--small btn--primary" data-action="brewday-hygiene-done" data-id="' +
									r.id +
									'">Mark hygiene done</button> ' +
									'<button type="button" class="btn btn--small" data-action="brewday-revoke" data-id="' +
									r.id +
									'">Remove brewday</button>';
							}
							if (r.status === "hygiene_done") {
								actions =
									'<button type="button" class="btn btn--small" data-action="brewday-edit" data-id="' +
									r.id +
									'">Edit</button> ' +
									'<button type="button" class="btn btn--small" data-action="brewday-hygiene-revoke" data-id="' +
									r.id +
									'">Revoke hygiene</button>';
							}
							return (
								"<tr><td>" +
								r.id +
								"</td><td>" +
								esc(r.name) +
								"</td><td>" +
								esc(r.brewery_name || "") +
								"</td><td>" +
								esc(r.status) +
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
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			const action = t.getAttribute("data-action");
			const id = t.getAttribute("data-id");
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
					alert(e.message);
				}
				return;
			}
			if (action === "brewday-revoke") {
				const ok = await appConfirm({
					title: "Remove brewday",
					message:
						"Undo brewday completed for recipe #" +
						id +
						"? OG and brew volume are cleared; the schedule booking stays.",
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/recipes/" + id + "/brewday/revoke", { method: "POST" });
					refresh();
				} catch (e) {
					alert(e.message);
				}
				return;
			}
			if (action === "brewday-hygiene-revoke") {
				const ok = await appConfirm({
					title: "Revoke hygiene",
					message:
						"Undo hygiene done for recipe #" +
						id +
						"? Status returns to brewday so you can edit OG/volume again.",
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/recipes/" + id + "/hygiene/revoke", { method: "POST" });
					refresh(id);
				} catch (e) {
					alert(e.message);
				}
				return;
			}
			if (action !== "brewday-hygiene-done") {
				return;
			}
			const ok = await appConfirm({
				title: "Hygiene complete",
				message: "Confirm that all hygiene routines are done for recipe #" + id + "?",
			});
			if (!ok) {
				return;
			}
			try {
				await api("/api/recipes/" + id + "/hygiene/complete", { method: "POST" });
				refresh();
			} catch (e) {
				alert(e.message);
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
		const defaultUnit = category === "hops" ? "g" : category === "yeast" ? "pack" : "kg";
		const list = panel.querySelector("#inventory-list");
		const dialog = panel.querySelector("#inventory-dialog");
		const form = panel.querySelector("#inventory-form");
		const titleEl = panel.querySelector("#inventory-dialog-title");
		const canEdit = canRoles(["superuser", "admin"]);

		panel.querySelectorAll(".inventory-field--malt").forEach((el) => {
			el.classList.toggle("is-role-hidden", !isMalt);
		});

		function fillForm(item) {
			form.reset();
			form.elements.namedItem("id").value = item && item.id ? String(item.id) : "";
			form.elements.namedItem("name").value = (item && item.name) || "";
			form.elements.namedItem("item_type").value = (item && item.item_type) || "";
			form.elements.namedItem("producer").value = (item && item.producer) || "";
			form.elements.namedItem("min_ebc").value = item && item.min_ebc != null ? item.min_ebc : 0;
			form.elements.namedItem("max_ebc").value = item && item.max_ebc != null ? item.max_ebc : 0;
			form.elements.namedItem("link").value = (item && item.link) || "";
			form.elements.namedItem("unit").value = (item && item.unit) || defaultUnit;
			form.elements.namedItem("qty").value = item && item.qty != null ? item.qty : 0;
			form.elements.namedItem("cost_price").value = item && item.cost_price != null ? item.cost_price : 0;
			if (titleEl) {
				titleEl.textContent = item && item.id ? "Edit item" : "Inventory item";
			}
		}

		function bodyFromForm() {
			const fd = new FormData(form);
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
				link: fd.get("link") || "",
			};
		}

		async function refresh() {
			list.textContent = "Loading…";
			try {
				const items = await api("/api/inventory?category=" + encodeURIComponent(category));
				if (!items || !items.length) {
					list.innerHTML = "<p class=\"panel__empty\">No items.</p>";
					return;
				}
				const headers = isMalt
					? ["Name", "Type", "Producer", "EBC", "Qty", "Cost", "", ""]
					: ["Name", "Type", "Producer", "Unit", "Qty", "Cost", "", ""];
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
								  '" target="_blank" rel="noopener noreferrer">Link</a>'
								: "";
							const orderBtn = canEdit
								? '<button type="button" class="btn btn--small" data-action="inventory-order" data-id="' +
								  i.id +
								  '" data-name="' +
								  esc(i.name) +
								  '">Add to order</button>'
								: "";
							const editBtn = canEdit
								? '<button type="button" class="btn btn--small" data-action="inventory-edit" data-id="' +
								  i.id +
								  '">Edit</button>'
								: "";
							if (isMalt) {
								return (
									"<tr><td>" +
									esc(i.name) +
									"</td><td>" +
									esc(i.item_type || "") +
									"</td><td>" +
									esc(i.producer || "") +
									"</td><td>" +
									ebc +
									"</td><td>" +
									i.qty +
									"</td><td>" +
									i.cost_price +
									"</td><td>" +
									linkCell +
									"</td><td>" +
									orderBtn +
									" " +
									editBtn +
									"</td></tr>"
								);
							}
							return (
								"<tr><td>" +
								esc(i.name) +
								"</td><td>" +
								esc(i.item_type || "") +
								"</td><td>" +
								esc(i.producer || "") +
								"</td><td>" +
								esc(i.unit) +
								"</td><td>" +
								i.qty +
								"</td><td>" +
								i.cost_price +
								"</td><td>" +
								linkCell +
								"</td><td>" +
								orderBtn +
								" " +
								editBtn +
								"</td></tr>"
							);
						})
						.join("")
				);
			} catch (e) {
				list.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			if (t.getAttribute("data-action") === "inventory-refresh") {
				refresh();
			}
			if (t.getAttribute("data-action") === "inventory-new") {
				if (!canEdit) {
					return;
				}
				fillForm(null);
				dialog.showModal();
			}
			if (t.getAttribute("data-action") === "inventory-edit") {
				if (!canEdit) {
					return;
				}
				const id = parseInt(t.getAttribute("data-id"), 10);
				api("/api/inventory/" + id)
					.then((item) => {
						fillForm(item);
						dialog.showModal();
					})
					.catch((e) => alert(e.message));
			}
			if (t.getAttribute("data-action") === "inventory-order") {
				if (!canEdit) {
					return;
				}
				const id = parseInt(t.getAttribute("data-id"), 10);
				const name = t.getAttribute("data-name") || "item";
				const values = await appPrompt({
					title: "Order qty",
					fields: [
						{
							name: "qty",
							label: "Qty to order for " + name,
							type: "number",
							step: "any",
							min: "0.01",
							value: "1",
						},
					],
				});
				if (!values) {
					return;
				}
				const qty = parseFloat(values.qty);
				if (!(qty > 0)) {
					alert("Qty must be positive");
					return;
				}
				api("/api/inventory/orders/planning/lines", {
					method: "POST",
					body: JSON.stringify({ inventory_item_id: id, qty }),
				})
					.then((order) => {
						alert("Added to order #" + order.id);
					})
					.catch((e) => alert(e.message));
			}
		});

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
				alert(e.message);
			}
		});
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
				const label = l.brewery_id != null ? l.brewery_name || "Brewery #" + l.brewery_id : "Unassigned";
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
				return "<p class=\"panel__empty\">No lines</p>";
			}
			const canEditLines =
				canManage && (order.status === "planning" || order.status === "ordered");
			return groupLinesByBrewery(lines)
				.map((g) => {
					const items = g.lines
						.map((l) => {
							const unit = l.unit ? " " + esc(l.unit) : "";
							const orderQty =
								l.ordered_qty != null && l.ordered_qty !== undefined
									? l.ordered_qty
									: l.qty;
							let text =
								esc(l.item_name) +
								" (" +
								esc(l.category) +
								") — need " +
								l.qty +
								unit +
								" · order " +
								orderQty +
								unit;
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
				linksTitle.textContent = "Product links — order #" + order.id;
			}
			const lines = order.lines || [];
			if (!lines.length) {
				linksList.innerHTML = "<p class=\"panel__empty\">No lines on this order.</p>";
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
						esc(l.category) +
						") — need " +
						l.qty +
						unit +
						" · order " +
						orderQty +
						unit;
					if (!l.inventory_item_id) {
						return (
							'<div class="order-link-row">' +
							'<p class="order-link-row__meta">' +
							meta +
							"</p>" +
							'<p class="panel__empty">No catalog item — link cannot be set.</p>' +
							"</div>"
						);
					}
					const linkVal = l.link || "";
					let actions =
						'<button type="button" class="btn btn--small" data-action="order-link-open" data-line-id="' +
						l.id +
						'">Open</button>';
					if (canManage) {
						actions +=
							' <button type="button" class="btn btn--small btn--primary" data-action="order-link-save" data-line-id="' +
							l.id +
							'">Save</button>';
					}
					return (
						'<div class="order-link-row" data-line-id="' +
						l.id +
						'">' +
						'<p class="order-link-row__meta">' +
						meta +
						"</p>" +
						'<label class="order-link-row__field">Product URL' +
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
				'<option value="">Unassigned</option>' +
				breweries
					.map((b) => '<option value="' + b.id + '">' + esc(b.name) + "</option>")
					.join("");
		}

		async function refresh() {
			list.textContent = "Loading…";
			try {
				const orders = await api("/api/inventory/orders");
				if (!orders || !orders.length) {
					list.innerHTML = "<p class=\"panel__empty\">No orders.</p>";
					return;
				}
				list.innerHTML = orders
					.map((o) => {
						const lineCount = (o.lines || []).length;
						let actions =
							'<button type="button" class="btn btn--small" data-action="order-product-links" data-id="' +
							o.id +
							'">Product links</button>';
						if (canManage && o.status !== "completed") {
							actions +=
								' <button type="button" class="btn btn--small" data-action="order-edit-external" data-id="' +
								o.id +
								'" data-external="' +
								esc(o.external_order_id || "") +
								'">Edit external ID</button>';
						}
						if (canManage && o.status === "planning") {
							actions +=
								' <button type="button" class="btn btn--small" data-action="order-add-line" data-id="' +
								o.id +
								'">Add line</button> ' +
								'<button type="button" class="btn btn--small btn--primary" data-action="order-status" data-id="' +
								o.id +
								'" data-status="ordered">Mark ordered</button>';
						}
						if (canManage && o.status === "ordered") {
							actions +=
								' <button type="button" class="btn btn--small btn--primary" data-action="order-status" data-id="' +
								o.id +
								'" data-status="completed">Mark completed</button>';
						}
						if (isAdmin && o.status === "planning" && lineCount === 0) {
							actions +=
								' <button type="button" class="btn btn--small" data-action="order-delete" data-id="' +
								o.id +
								'">Delete</button>';
						}
						const ext =
							o.external_order_id && String(o.external_order_id).trim()
								? '<p class="order-external-id">External ID: ' + esc(o.external_order_id) + "</p>"
								: '<p class="order-external-id order-external-id--empty">No external ID</p>';
						let dates =
							'<p class="order-dates">Created: ' +
							esc(formatOrderDate(o.created_at)) +
							" · Updated: " +
							esc(formatOrderDate(o.updated_at || o.created_at));
						if (o.ordered_at) {
							dates += " · Ordered: " + esc(formatOrderDate(o.ordered_at));
						}
						dates += "</p>";
						return (
							'<div class="panel__card" data-order-id="' +
							o.id +
							'"><div class="panel__card-head"><strong>#' +
							o.id +
							'</strong> <span class="order-status order-status--' +
							esc(o.status) +
							'">' +
							esc(o.status) +
							"</span></div>" +
							dates +
							ext +
							(o.notes ? "<p>" + esc(o.notes) + "</p>" : "") +
							renderLines(o, o.lines) +
							'<div class="panel__card-actions">' +
							actions +
							"</div>" +
							"</div>"
						);
					})
					.join("");
			} catch (e) {
				list.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			if (t.getAttribute("data-action") === "orders-refresh") {
				refresh();
			}
			if (t.getAttribute("data-action") === "orders-new") {
				if (!canManage) {
					return;
				}
				newForm.reset();
				newDialog.showModal();
			}
			if (t.getAttribute("data-action") === "order-product-links") {
				try {
					await openLinksModal(t.getAttribute("data-id"));
				} catch (e) {
					alert(e.message);
				}
			}
			if (t.getAttribute("data-action") === "order-link-open") {
				const row = t.closest(".order-link-row");
				const input = row && row.querySelector('input[name="link"]');
				const url = input ? String(input.value || "").trim() : "";
				if (!url) {
					alert("No product URL set");
					return;
				}
				window.open(url, "_blank", "noopener,noreferrer");
			}
			if (t.getAttribute("data-action") === "order-link-save") {
				if (!canManage) {
					return;
				}
				const lineId = t.getAttribute("data-line-id");
				const orderId = linksOrderIdEl && linksOrderIdEl.value;
				const row = t.closest(".order-link-row");
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
					alert(e.message);
				}
			}
			if (t.getAttribute("data-action") === "order-add-line") {
				if (!canManage) {
					return;
				}
				try {
					await loadItemOptions();
					await loadBreweryOptions();
					lineForm.elements.namedItem("order_id").value = t.getAttribute("data-id");
					lineDialog.showModal();
				} catch (e) {
					alert(e.message);
				}
			}
			if (t.getAttribute("data-action") === "order-edit-external") {
				if (!canManage) {
					return;
				}
				externalForm.elements.namedItem("order_id").value = t.getAttribute("data-id");
				externalForm.elements.namedItem("external_order_id").value =
					t.getAttribute("data-external") || "";
				externalDialog.showModal();
			}
			if (t.getAttribute("data-action") === "order-line-ordered-qty") {
				if (!canManage) {
					return;
				}
				const defaultQty = t.getAttribute("data-ordered") || t.getAttribute("data-need") || "1";
				orderedQtyForm.elements.namedItem("order_id").value = t.getAttribute("data-order-id");
				orderedQtyForm.elements.namedItem("line_id").value = t.getAttribute("data-line-id");
				orderedQtyForm.elements.namedItem("ordered_qty").value = defaultQty;
				orderedQtyDialog.showModal();
			}
			if (t.getAttribute("data-action") === "order-status") {
				if (!canManage) {
					return;
				}
				const status = t.getAttribute("data-status");
				confirmForm.elements.namedItem("action").value = "status";
				confirmForm.elements.namedItem("order_id").value = t.getAttribute("data-id");
				confirmForm.elements.namedItem("status").value = status;
				confirmTitle.textContent = status === "completed" ? "Complete order" : "Mark ordered";
				confirmMessage.textContent =
					status === "completed"
						? "Complete and receive stock into inventory?"
						: "Mark this order as ordered?";
				confirmDialog.showModal();
			}
			if (t.getAttribute("data-action") === "order-delete") {
				if (!isAdmin) {
					return;
				}
				confirmForm.elements.namedItem("action").value = "delete";
				confirmForm.elements.namedItem("order_id").value = t.getAttribute("data-id");
				confirmForm.elements.namedItem("status").value = "";
				confirmTitle.textContent = "Delete order";
				confirmMessage.textContent = "Delete this empty order?";
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
				alert("Created order #" + order.id);
				refresh();
			} catch (e) {
				alert(e.message);
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
				alert(e.message);
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
				alert(e.message);
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
				alert(e.message);
			}
		});

		orderedQtyDialog.addEventListener("close", async () => {
			if (orderedQtyDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(orderedQtyForm);
			const orderedQty = parseFloat(fd.get("ordered_qty"));
			if (!(orderedQty > 0)) {
				alert("Ordered qty must be positive");
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
				alert(e.message);
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
				return "<p class=\"panel__empty\">No hygiene routines.</p>";
			}
			const groups = [];
			const index = {};
			routines.forEach((r) => {
				const section = r.description || "Checklist";
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
			list.textContent = "Loading…";
			try {
				const [routines, recipes] = await Promise.all([
					api("/api/settings/hygiene-routines"),
					api("/api/recipes"),
				]);
				list.innerHTML = renderGroupedRoutines(routines || []);
				const brewday = (recipes || []).filter((r) => r.status === "brewday");
				fillRecipeSelect(recipeSel, brewday, "No brewday recipes");
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
				title: "Hygiene complete",
				message:
					"Confirm that all hygiene routines are done for recipe #" + recipeId + "?",
			});
			if (!ok) {
				return;
			}
			try {
				await api("/api/recipes/" + recipeId + "/hygiene/complete", { method: "POST" });
				refresh();
			} catch (e) {
				errEl.hidden = false;
				errEl.textContent = e.message;
			}
		});
		refresh();
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
						" kr/L</td></tr>"
					);
				})
				.join("");
			examplesEl.innerHTML =
				"<p class=\"panel__lead\">Examples at current settings (ABV × " +
				rate +
				" × " +
				discount +
				"):</p>" +
				table(["Öl ABV", "Alkoholskatt"], rows);
		}

		async function refresh() {
			examplesEl.textContent = "Loading…";
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
		let rows = [];

		const columnHelp = {
			delivered: {
				title: "Delivered",
				message: "Date the batch was marked delivered.",
			},
			id: {
				title: "ID",
				message: "Internal recipe/batch id.",
			},
			name: {
				title: "Name",
				message: "Batch name.",
			},
			brewery: {
				title: "Brewery",
				message: "Brewery that produced the batch.",
			},
			abv: {
				title: "ABV",
				message: "Alcohol by volume from (OG − FG) × 131.25.",
			},
			volume: {
				title: "Volume (L)",
				message: "Liters delivered to the pub.",
			},
			cost: {
				title: "Cost",
				message: "Sum of ingredient qty × cost price.",
			},
			tax: {
				title: "Tax",
				message: "Alcohol tax for this delivery volume.",
			},
			net: {
				title: "Net",
				message:
					"Invoice net: beer net × multiplier × volume. Tax is separate and not included.",
			},
			"net-per-l": {
				title: "Net SEK/L",
				message: "Beer net × multiplier (net divided by delivery volume).",
			},
		};

		const helpHeaders = [
			{ key: "delivered", label: "Delivered" },
			{ key: "id", label: "ID" },
			{ key: "name", label: "Name" },
			{ key: "brewery", label: "Brewery" },
			{ key: "abv", label: "ABV" },
			{ key: "volume", label: "Volume (L)" },
			{ key: "cost", label: "Cost" },
			{ key: "tax", label: "Tax" },
			{ key: "net", label: "Net" },
			{ key: "net-per-l", label: "Net SEK/L" },
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
				return;
			}
			sumVol.textContent = fmtMoney(totals.volume) + " L";
			sumCost.textContent = fmtMoney(totals.cost);
			sumTax.textContent = fmtMoney(totals.tax);
			sumNet.textContent = fmtMoney(totals.net);
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
				"Delivered",
				"ID",
				"Name",
				"Brewery",
				"ABV %",
				"Volume L",
				"Cost",
				"Tax",
				"Net",
				"Net SEK/L",
			];
			const lines = [header.join(",")];
			rows.forEach((r) => {
				const abv =
					r.og != null && r.fg != null
						? ((r.og - r.fg) * 131.25).toFixed(1)
						: "";
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
			list.textContent = "Loading…";
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
				const totals = { volume: 0, cost: 0, tax: 0, net: 0 };
				rows.forEach((r) => {
					totals.volume += Number(r.delivery_volume) || 0;
					totals.cost += Number(r.cost) || 0;
					totals.tax += Number(r.tax) || 0;
					totals.net += Number(r.net) || 0;
				});
				setSummary(totals);
				if (!rows.length) {
					list.innerHTML =
						'<p class="panel__empty">No deliveries in ' + esc(month) + ".</p>";
					return;
				}
				list.innerHTML = helpTable(
					rows
						.map(
							(r) =>
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
								fmtMoney(r.delivery_volume) +
								"</td><td>" +
								fmtMoney(r.cost) +
								"</td><td>" +
								fmtMoney(r.tax) +
								"</td><td>" +
								fmtMoney(r.net) +
								"</td><td>" +
								esc(netPerLiter(r)) +
								"</td></tr>"
						)
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
				fillRecipeSelect(recipeSel, selectable, "No recipes ready for delivery calc");
				if (prev && byID[prev]) {
					recipeSel.value = prev;
				}
				fillFG(byID[recipeSel.value]);
				await loadRecipePreview(recipeSel.value);
				const relevant = (recipes || []).filter((r) =>
					["hygiene_done", "ready_for_delivery", "delivered"].includes(r.status)
				);
				if (!relevant.length) {
					list.innerHTML = "<p class=\"panel__empty\">No batches in delivery pipeline.</p>";
					return;
				}
				list.innerHTML = table(
					[
						"ID",
						"Name",
						"Brewery",
						"Status",
						"Brew date",
						"Delivery date",
						"ABV",
						"Cost",
						"Tax",
						"Net",
						"",
					],
					relevant
						.map((r) => {
							let btn = "";
							if (r.status === "ready_for_delivery") {
								btn =
									'<button type="button" class="btn btn--small" data-deliver="' +
									r.id +
									'">Deliver</button>';
							} else if (r.status === "delivered") {
								btn =
									'<button type="button" class="btn btn--small" data-revoke-delivery="' +
									r.id +
									'">Revoke</button>';
							}
							return (
								"<tr><td>" +
								r.id +
								"</td><td>" +
								esc(r.name) +
								"</td><td>" +
								esc(r.brewery_name || "") +
								"</td><td>" +
								esc(r.status) +
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
								btn +
								"</td></tr>"
							);
						})
						.join("")
				);
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
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			if (t.hasAttribute("data-deliver")) {
				try {
					await api("/api/recipes/" + t.getAttribute("data-deliver") + "/deliver", {
						method: "POST",
					});
					refresh();
				} catch (e) {
					alert(e.message);
				}
				return;
			}
			if (t.hasAttribute("data-revoke-delivery")) {
				const id = t.getAttribute("data-revoke-delivery");
				const ok = await appConfirm({
					title: "Revoke delivery",
					message:
						"Revoke delivery for recipe #" +
						id +
						"? This clears FG, delivery volume, and pricing so you can recalculate.",
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/recipes/" + id + "/delivery/revoke", { method: "POST" });
					refresh();
				} catch (e) {
					alert(e.message);
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
					["ID", "Username", "Email", "Role", "Active", ""],
					(users || [])
						.map((u) => {
							const active = u.active !== false;
							const label = active ? "Active" : "Inactive";
							const btnLabel = active ? "Deactivate" : "Activate";
							const btn =
								'<button type="button" class="btn btn--small" data-edit-user="' +
								u.id +
								'">Edit</button> ' +
								'<button type="button" class="btn btn--small" data-set-active="' +
								u.id +
								'" data-active="' +
								(active ? "0" : "1") +
								'">' +
								btnLabel +
								'</button> <button type="button" class="btn btn--small" data-reset-password="' +
								u.id +
								'">Reset password</button>';
							return (
								"<tr><td>" +
								u.id +
								"</td><td>" +
								esc(u.username) +
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
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			const action = t.getAttribute("data-action");
			if (action === "iam-users-refresh") {
				refreshUsers();
			}
			if (action === "iam-user-new") {
				const dlg = panel.querySelector("#iam-user-dialog");
				const brewSel = dlg.querySelector('[name="brewery_id"]');
				try {
					const breweries = await api("/api/breweries");
					brewSel.innerHTML =
						'<option value="">None</option>' +
						(breweries || [])
							.map((b) => '<option value="' + b.id + '">' + esc(b.name) + "</option>")
							.join("");
				} catch (e) {
					brewSel.innerHTML = '<option value="">None</option>';
				}
				dlg.showModal();
			}
			if (t.hasAttribute("data-edit-user")) {
				const id = t.getAttribute("data-edit-user");
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
					roleSel.value = user.role || "user";
					const editingSelfAdmin =
						String(user.id) === selfID && (user.role || "") === "admin";
					roleSel.disabled = editingSelfAdmin;
					hint.hidden = !editingSelfAdmin;
					dlg.showModal();
				} catch (e) {
					alert(e.message);
				}
			}
			if (t.hasAttribute("data-set-active")) {
				const id = t.getAttribute("data-set-active");
				const active = t.getAttribute("data-active") === "1";
				try {
					await api("/api/users/" + id + "/active", {
						method: "PATCH",
						body: JSON.stringify({ active }),
					});
					refreshUsers();
				} catch (e) {
					alert(e.message);
				}
			}
			if (t.hasAttribute("data-reset-password")) {
				const id = t.getAttribute("data-reset-password");
				const dlg = panel.querySelector("#iam-reset-password-dialog");
				const form = panel.querySelector("#iam-reset-password-form");
				try {
					const user = await api("/api/users/" + id);
					form.querySelector('[name="user_id"]').value = user.id;
					form.querySelector('[name="username"]').value = user.username || "";
					form.querySelector('[name="email"]').value = user.email || "";
					form.querySelector('[name="role"]').value = user.role || "user";
					form.querySelector('[name="password"]').value = "";
					dlg.showModal();
				} catch (e) {
					alert(e.message);
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
			const body = {
				username: fd.get("username"),
				password: fd.get("password"),
				email: fd.get("email"),
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
				alert(e.message);
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
				alert("cannot change your own role away from admin");
				return;
			}
			try {
				await api("/api/users/" + id, {
					method: "PATCH",
					body: JSON.stringify({
						username: fd.get("username"),
						email: fd.get("email"),
						role: role,
					}),
				});
				editUserForm.reset();
				roleSel.disabled = false;
				panel.querySelector("#iam-edit-role-hint").hidden = true;
				refreshUsers();
			} catch (e) {
				alert(e.message);
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
			try {
				await api("/api/users/" + id, {
					method: "PATCH",
					body: JSON.stringify({
						username: fd.get("username"),
						email: fd.get("email"),
						role: fd.get("role"),
						password: fd.get("password"),
					}),
				});
				resetForm.reset();
				alert("Password updated");
			} catch (e) {
				alert(e.message);
			}
		});

		refreshUsers();
	}

	async function loadIAMBreweries(panel) {
		const brewEl = panel.querySelector("#iam-breweries");

		async function refreshBreweries() {
			try {
				const list = await api("/api/breweries");
				brewEl.innerHTML = (list || [])
					.map(
						(b) =>
							'<div class="panel__card"><strong>#' +
							b.id +
							" " +
							esc(b.name) +
							"</strong><br>" +
							esc(b.contact_name) +
							" " +
							esc(b.contact_email) +
							" " +
							esc(b.contact_phone || "") +
							'<br><button type="button" class="btn btn--small" data-edit-brewery="' +
							b.id +
							'">Edit</button> <button type="button" class="btn btn--small" data-del-brewery="' +
							b.id +
							'" data-brewery-name="' +
							esc(b.name) +
							'">Remove</button> <button type="button" class="btn btn--small" data-add-member="' +
							b.id +
							'" data-brewery-name="' +
							esc(b.name) +
							'">Add member</button> <button type="button" class="btn btn--small" data-list-members="' +
							b.id +
							'" data-brewery-name="' +
							esc(b.name) +
							'">Members</button></div>'
					)
					.join("") || '<p class="panel__empty">No breweries.</p>';
			} catch (e) {
				brewEl.textContent = e.message;
			}
		}

		async function fillBreweryAdminSelect(selectEl, selectedID) {
			try {
				const users = await api("/api/users");
				selectEl.innerHTML =
					'<option value="">None</option>' +
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
				selectEl.innerHTML = '<option value="">None</option>';
			}
		}

		async function openBreweryDialog(breweryID) {
			const dlg = panel.querySelector("#iam-brewery-dialog");
			const form = panel.querySelector("#iam-brewery-form");
			const title = panel.querySelector("#iam-brewery-title");
			const adminSel = form.querySelector('[name="brewery_admin_user_id"]');
			form.reset();
			form.querySelector('[name="brewery_id"]').value = breweryID ? String(breweryID) : "";
			let adminID = "";
			if (breweryID) {
				title.textContent = "Edit brewery";
				const [brewery, members] = await Promise.all([
					api("/api/breweries/" + breweryID),
					api("/api/breweries/" + breweryID + "/members"),
				]);
				form.querySelector('[name="name"]').value = brewery.name || "";
				form.querySelector('[name="contact_name"]').value = brewery.contact_name || "";
				form.querySelector('[name="contact_email"]').value = brewery.contact_email || "";
				form.querySelector('[name="contact_phone"]').value = brewery.contact_phone || "";
				const admin = (members || []).find((m) => m.role === "brewery_admin");
				if (admin) {
					adminID = String(admin.user_id);
				}
			} else {
				title.textContent = "New brewery";
			}
			await fillBreweryAdminSelect(adminSel, adminID);
			dlg.showModal();
		}

		const membersDialog = panel.querySelector("#iam-members-dialog");
		const membersClose = panel.querySelector("#iam-members-close");

		async function refreshMembersList(breweryID) {
			const listEl = panel.querySelector("#iam-members-list");
			listEl.textContent = "Loading…";
			try {
				const members = await api("/api/breweries/" + breweryID + "/members");
				if (!members || !members.length) {
					listEl.innerHTML = '<p class="panel__empty">No members.</p>';
					return;
				}
				const roles = ["user", "superuser", "brewery_admin"];
				listEl.innerHTML = table(
					["User", "Role", ""],
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
								'">' +
								opts +
								'</select></td><td><button type="button" class="btn btn--small" data-remove-member="' +
								m.user_id +
								'">Remove</button></td></tr>'
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
			panel.querySelector("#iam-members-title").textContent = "Members — " + breweryName;
			await refreshMembersList(breweryID);
			membersDialog.showModal();
		}

		if (membersClose) {
			membersClose.addEventListener("click", () => {
				membersDialog.close();
			});
		}

		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			const action = t.getAttribute("data-action");
			if (action === "iam-breweries-refresh") {
				refreshBreweries();
			}
			if (action === "iam-brewery-new") {
				try {
					await openBreweryDialog(null);
				} catch (e) {
					alert(e.message);
				}
			}
			if (t.hasAttribute("data-edit-brewery")) {
				try {
					await openBreweryDialog(t.getAttribute("data-edit-brewery"));
				} catch (e) {
					alert(e.message);
				}
				return;
			}
			if (t.hasAttribute("data-del-brewery")) {
				const id = t.getAttribute("data-del-brewery");
				const name = t.getAttribute("data-brewery-name") || id;
				const ok = await appConfirm({
					title: "Remove brewery",
					message:
						"Remove brewery #" +
						id +
						" (" +
						name +
						")? This is blocked if the brewery has delivered batches.",
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/breweries/" + id, { method: "DELETE" });
					refreshBreweries();
				} catch (e) {
					alert(e.message);
				}
				return;
			}
			if (t.hasAttribute("data-add-member")) {
				const breweryID = t.getAttribute("data-add-member");
				const dlg = panel.querySelector("#iam-member-dialog");
				const form = panel.querySelector("#iam-member-form");
				const userSel = form.querySelector('[name="user_id"]');
				form.querySelector('[name="brewery_id"]').value = breweryID;
				userSel.innerHTML = '<option value="">Select user…</option>';
				try {
					const [users, members] = await Promise.all([
						api("/api/users"),
						api("/api/breweries/" + breweryID + "/members"),
					]);
					const memberIDs = new Set((members || []).map((m) => m.user_id));
					userSel.innerHTML =
						'<option value="">Select user…</option>' +
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
					alert(e.message);
					return;
				}
				dlg.showModal();
			}
			if (t.hasAttribute("data-list-members")) {
				const breweryID = t.getAttribute("data-list-members");
				const name = t.getAttribute("data-brewery-name") || "#" + breweryID;
				openMembersDialog(breweryID, name);
			}
			if (t.hasAttribute("data-remove-member")) {
				const breweryID = panel.querySelector("#iam-members-brewery-id").value;
				const userID = t.getAttribute("data-remove-member");
				const ok = await appConfirm({
					title: "Remove member",
					message: "Remove this member from the brewery?",
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
					alert(e.message);
				}
			}
		});

		panel.addEventListener("change", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			if (!t.hasAttribute("data-member-role")) {
				return;
			}
			const breweryID = panel.querySelector("#iam-members-brewery-id").value;
			const userID = parseInt(t.getAttribute("data-member-role"), 10);
			try {
				await api("/api/breweries/" + breweryID + "/members", {
					method: "POST",
					body: JSON.stringify({ user_id: userID, role: t.value }),
				});
			} catch (e) {
				alert(e.message);
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
			};
			if (adminID) {
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
				alert(e.message);
			}
		});

		const memberDialog = panel.querySelector("#iam-member-dialog");
		const memberForm = panel.querySelector("#iam-member-form");
		memberDialog.addEventListener("close", async () => {
			if (memberDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(memberForm);
			try {
				await api("/api/breweries/" + fd.get("brewery_id") + "/members", {
					method: "POST",
					body: JSON.stringify({
						user_id: parseInt(fd.get("user_id"), 10),
						role: fd.get("role"),
					}),
				});
				alert("Member added");
			} catch (e) {
				alert(e.message);
			}
		});

		refreshBreweries();
	}

	async function loadSettingsBrand(panel) {
		const logoPreview = panel.querySelector("#settings-logo-preview");
		const faviconPreview = panel.querySelector("#settings-favicon-preview");
		const logoErr = panel.querySelector("#settings-logo-error");
		const faviconErr = panel.querySelector("#settings-favicon-error");

		function bustBrand(kind) {
			const url = "/" + kind + "?t=" + Date.now();
			if (kind === "logo") {
				if (logoPreview) {
					logoPreview.src = url;
				}
				document.querySelectorAll(".splash__logo, .login__logo, .shell__brand-logo").forEach((img) => {
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
				errEl.textContent = "Choose a file first";
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

		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			const action = t.getAttribute("data-action");
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
		});
	}

	async function loadSettingsTanks(panel) {
		const tanksEl = panel.querySelector("#settings-tanks");

		async function refresh() {
			try {
				const tanks = await api("/api/settings/tanks");
				tanksEl.innerHTML = table(
					["ID", "Name", "Capacity L", "Active", ""],
					(tanks || [])
						.map((t) => {
							const toggleLabel = t.active ? "Disable" : "Enable";
							return (
								"<tr><td>" +
								t.id +
								"</td><td>" +
								esc(t.name) +
								"</td><td>" +
								t.capacity_liters +
								"</td><td>" +
								(t.active ? "yes" : "no") +
								'</td><td><button type="button" class="btn btn--small" data-tank-edit="' +
								t.id +
								'" data-name="' +
								esc(t.name) +
								'" data-capacity="' +
								t.capacity_liters +
								'">Edit</button> <button type="button" class="btn btn--small" data-tank-active="' +
								t.id +
								'" data-active="' +
								(t.active ? "0" : "1") +
								'">' +
								toggleLabel +
								'</button> <button type="button" class="btn btn--small" data-tank-delete="' +
								t.id +
								'">Remove</button></td></tr>'
							);
						})
						.join("")
				);
			} catch (e) {
				tanksEl.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			if (t.hasAttribute("data-tank-edit")) {
				const id = t.getAttribute("data-tank-edit");
				const values = await appPrompt({
					title: "Edit tank",
					fields: [
						{
							name: "name",
							label: "Tank name",
							type: "text",
							value: t.getAttribute("data-name") || "",
						},
						{
							name: "capacity_liters",
							label: "Capacity liters",
							type: "number",
							step: "any",
							value: t.getAttribute("data-capacity") || "0",
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
					alert(e.message);
				}
				return;
			}
			if (t.hasAttribute("data-tank-active")) {
				try {
					await api("/api/settings/tanks/" + t.getAttribute("data-tank-active") + "/active", {
						method: "POST",
						body: JSON.stringify({ active: t.getAttribute("data-active") === "1" }),
					});
					refresh();
				} catch (e) {
					alert(e.message);
				}
				return;
			}
			if (t.hasAttribute("data-tank-delete")) {
				const id = t.getAttribute("data-tank-delete");
				const ok = await appConfirm({
					title: "Remove tank",
					message: "Remove tank #" + id + " permanently?",
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/settings/tanks/" + id, { method: "DELETE" });
					refresh();
				} catch (e) {
					alert(e.message);
				}
				return;
			}
			if (t.getAttribute("data-action") !== "settings-tank-new") {
				return;
			}
			const values = await appPrompt({
				title: "New tank",
				fields: [
					{ name: "name", label: "Tank name", type: "text" },
					{ name: "capacity_liters", label: "Capacity liters", type: "number", step: "any", value: "1000" },
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
				alert(e.message);
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
					["ID", "Name", "×", "Active", ""],
					(mults || [])
						.map((m) => {
							const toggleLabel = m.active ? "Disable" : "Enable";
							return (
								"<tr><td>" +
								m.id +
								"</td><td>" +
								esc(m.name) +
								"</td><td>" +
								m.multiplier +
								"</td><td>" +
								(m.active ? "yes" : "no") +
								'</td><td><button type="button" class="btn btn--small" data-mult-edit="' +
								m.id +
								'" data-name="' +
								esc(m.name) +
								'" data-multiplier="' +
								m.multiplier +
								'">Edit</button> <button type="button" class="btn btn--small" data-mult-active="' +
								m.id +
								'" data-active="' +
								(m.active ? "0" : "1") +
								'">' +
								toggleLabel +
								'</button> <button type="button" class="btn btn--small" data-mult-delete="' +
								m.id +
								'">Remove</button></td></tr>'
							);
						})
						.join("")
				);
			} catch (e) {
				multsEl.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			if (t.hasAttribute("data-mult-edit")) {
				const id = t.getAttribute("data-mult-edit");
				const values = await appPrompt({
					title: "Edit multiplier",
					fields: [
						{
							name: "name",
							label: "Multiplier name",
							type: "text",
							value: t.getAttribute("data-name") || "",
						},
						{
							name: "multiplier",
							label: "Multiplier",
							type: "number",
							step: "any",
							value: t.getAttribute("data-multiplier") || "1",
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
					alert(e.message);
				}
				return;
			}
			if (t.hasAttribute("data-mult-active")) {
				try {
					await api(
						"/api/settings/multipliers/" + t.getAttribute("data-mult-active") + "/active",
						{
							method: "POST",
							body: JSON.stringify({ active: t.getAttribute("data-active") === "1" }),
						}
					);
					refresh();
				} catch (e) {
					alert(e.message);
				}
				return;
			}
			if (t.hasAttribute("data-mult-delete")) {
				const id = t.getAttribute("data-mult-delete");
				const ok = await appConfirm({
					title: "Remove multiplier",
					message: "Remove multiplier #" + id + " permanently?",
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/settings/multipliers/" + id, { method: "DELETE" });
					refresh();
				} catch (e) {
					alert(e.message);
				}
				return;
			}
			if (t.getAttribute("data-action") !== "settings-mult-new") {
				return;
			}
			const values = await appPrompt({
				title: "New multiplier",
				fields: [
					{ name: "name", label: "Multiplier name", type: "text" },
					{ name: "multiplier", label: "Multiplier", type: "number", step: "any", value: "1.5" },
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
				alert(e.message);
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

	async function loadSettingsHygiene(panel) {
		const hygEl = panel.querySelector("#settings-hygiene");

		async function refresh() {
			try {
				const hyg = await api("/api/settings/hygiene-routines");
				hygEl.innerHTML = table(
					["Name", "Description", "Sort", ""],
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
								'">Edit</button> <button type="button" class="btn btn--small" data-hygiene-delete="' +
								r.id +
								'">Remove</button></td></tr>'
							);
						})
						.join("")
				);
			} catch (e) {
				hygEl.textContent = e.message;
			}
		}

		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			if (t.hasAttribute("data-hygiene-edit")) {
				const id = t.getAttribute("data-hygiene-edit");
				const values = await appPrompt({
					title: "Edit hygiene routine",
					fields: [
						{
							name: "name",
							label: "Name",
							type: "text",
							value: t.getAttribute("data-name") || "",
						},
						{
							name: "description",
							label: "Description",
							type: "text",
							value: t.getAttribute("data-description") || "",
						},
						{
							name: "sort_order",
							label: "Sort order",
							type: "number",
							step: "1",
							value: t.getAttribute("data-sort") || "0",
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
					alert(e.message);
				}
				return;
			}
			if (t.hasAttribute("data-hygiene-delete")) {
				const id = t.getAttribute("data-hygiene-delete");
				const ok = await appConfirm({
					title: "Remove hygiene routine",
					message: "Remove routine #" + id + " permanently?",
				});
				if (!ok) {
					return;
				}
				try {
					await api("/api/settings/hygiene-routines/" + id, { method: "DELETE" });
					refresh();
				} catch (e) {
					alert(e.message);
				}
				return;
			}
			if (t.getAttribute("data-action") === "settings-hygiene-refresh") {
				refresh();
				return;
			}
			if (t.getAttribute("data-action") !== "settings-hygiene-new") {
				return;
			}
			const values = await appPrompt({
				title: "New hygiene routine",
				fields: [
					{ name: "name", label: "Name", type: "text" },
					{ name: "description", label: "Description", type: "text" },
					{ name: "sort_order", label: "Sort order", type: "number", step: "1", value: "0" },
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
				alert(e.message);
			}
		});
		refresh();
	}
})();

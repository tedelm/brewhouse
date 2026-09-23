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
			'<table class="data-table"><thead><tr>' +
			headers.map((h) => "<th>" + esc(h) + "</th>").join("") +
			"</tr></thead><tbody>" +
			rowsHtml +
			"</tbody></table>"
		);
	}

	function fmtMoney(n) {
		if (n == null || n === "" || Number.isNaN(Number(n))) {
			return "";
		}
		return Number(n).toFixed(2);
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
		if ((kind === "iam" || kind === "settings") && !canRoles("admin")) {
			showForbidden(panel);
			return;
		}
		if (kind === "economy" && !canRoles(["superuser", "admin"])) {
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
			delivery: loadDelivery,
			iam: loadIAM,
			settings: loadSettings,
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

		function editableStatus(status) {
			return status === "created" || status === "scheduled";
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
				const recipes = await api("/api/recipes");
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
							}
							actions +=
								'<button type="button" class="btn btn--small" data-del-recipe="' +
								r.id +
								'">Delete</button>';
							if (r.status === "scheduled") {
								actions +=
									' <button type="button" class="btn btn--small" data-brewday="' +
									r.id +
									'">Brewday</button>';
							}
							if (r.status === "ready_for_delivery") {
								actions +=
									' <button type="button" class="btn btn--small" data-deliver="' +
									r.id +
									'">Deliver</button>';
							}
							return (
								"<tr><td>" +
								r.id +
								"</td><td>" +
								esc(r.name) +
								"</td><td>" +
								esc(r.brewery_name || String(r.brewery_id)) +
								"</td><td>" +
								esc(r.status) +
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
			if (t.hasAttribute("data-brewday")) {
				const id = t.getAttribute("data-brewday");
				const values = await appPrompt({
					title: "Brewday",
					fields: [
						{ name: "og", label: "OG (SG)", type: "number", step: "0.001", value: "1.050" },
						{ name: "brew_volume", label: "Brew volume (L)", type: "number", step: "any", value: "100" },
					],
				});
				if (!values) {
					return;
				}
				try {
					await api("/api/recipes/" + id + "/brewday", {
						method: "POST",
						body: JSON.stringify({
							og: parseFloat(values.og),
							brew_volume: parseFloat(values.brew_volume),
						}),
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
					api("/api/settings/tanks"),
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
					["Date", "End date", "Brewery", "Recipe", "Tank"],
					bookings
						.map(
							(b) =>
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
								"</td></tr>"
						)
						.join("")
				);
			} catch (e) {
				list.textContent = e.message;
			}
		}

		panel.querySelector('[data-action="schedule-refresh"]').addEventListener("click", refresh);
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
			if (recipe && recipe.og != null && recipe.og !== "") {
				ogInput.value = recipe.og;
			} else {
				ogInput.value = "1.050";
			}
			if (recipe && recipe.brew_volume != null && recipe.brew_volume !== "") {
				volInput.value = recipe.brew_volume;
			} else {
				volInput.value = "100";
			}
		}

		async function refresh() {
			list.textContent = "Loading…";
			try {
				const recipes = await api("/api/recipes");
				const selectable = (recipes || []).filter(
					(r) => r.status === "scheduled" || r.status === "brewday"
				);
				byID = {};
				selectable.forEach((r) => {
					byID[r.id] = r;
				});
				const prev = recipeSel.value;
				fillRecipeSelect(recipeSel, selectable, "No batches ready");
				if (prev && byID[prev]) {
					recipeSel.value = prev;
				}
				fillBrewdayFields(byID[recipeSel.value]);
				if (!selectable.length) {
					list.innerHTML = "<p class=\"panel__empty\">No scheduled or brewday batches.</p>";
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
									'">Mark hygiene done</button>';
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
								(r.og ?? "") +
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

		panel.querySelector('[data-action="brewday-refresh"]').addEventListener("click", refresh);
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
				if (byID[id]) {
					recipeSel.value = id;
					fillBrewdayFields(byID[id]);
					form.scrollIntoView({ behavior: "smooth", block: "nearest" });
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
						let actions = "";
						if (canManage && o.status !== "completed") {
							actions +=
								'<button type="button" class="btn btn--small" data-action="order-edit-external" data-id="' +
								o.id +
								'" data-external="' +
								esc(o.external_order_id || "") +
								'">Edit external ID</button> ';
						}
						if (canManage && o.status === "planning") {
							actions +=
								'<button type="button" class="btn btn--small" data-action="order-add-line" data-id="' +
								o.id +
								'">Add line</button> ' +
								'<button type="button" class="btn btn--small btn--primary" data-action="order-status" data-id="' +
								o.id +
								'" data-status="ordered">Mark ordered</button>';
						}
						if (canManage && o.status === "ordered") {
							actions +=
								'<button type="button" class="btn btn--small btn--primary" data-action="order-status" data-id="' +
								o.id +
								'" data-status="completed">Mark completed</button>';
						}
						if (isAdmin && o.status === "planning" && lineCount === 0) {
							actions +=
								(actions ? " " : "") +
								'<button type="button" class="btn btn--small" data-action="order-delete" data-id="' +
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
							(actions ? '<div class="panel__card-actions">' + actions + "</div>" : "") +
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

	async function loadDelivery(panel) {
		const list = panel.querySelector("#delivery-list");
		const form = panel.querySelector("#delivery-form");
		const errEl = panel.querySelector("#delivery-error");
		const recipeSel = panel.querySelector("#delivery-recipe");
		async function refresh() {
			try {
				const recipes = await api("/api/recipes");
				const selectable = (recipes || []).filter(
					(r) => r.status === "hygiene_done" || r.status === "ready_for_delivery"
				);
				fillRecipeSelect(recipeSel, selectable, "No recipes ready for delivery calc");
				const relevant = (recipes || []).filter((r) =>
					["hygiene_done", "ready_for_delivery", "delivered", "brewday"].includes(r.status)
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

	async function loadIAM(panel) {
		const usersEl = panel.querySelector("#iam-users");
		const brewEl = panel.querySelector("#iam-breweries");

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
							'</strong><br>' +
							esc(b.contact_name) +
							" " +
							esc(b.contact_email) +
							'<br><button type="button" class="btn btn--small" data-add-member="' +
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

		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			const action = t.getAttribute("data-action");
			if (action === "iam-users-refresh") {
				refreshUsers();
			}
			if (action === "iam-breweries-refresh") {
				refreshBreweries();
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
			if (action === "iam-brewery-new") {
				const dlg = panel.querySelector("#iam-brewery-dialog");
				const adminSel = dlg.querySelector('[name="brewery_admin_user_id"]');
				try {
					const users = await api("/api/users");
					adminSel.innerHTML =
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
				} catch (e) {
					adminSel.innerHTML = '<option value="">None</option>';
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

		const breweryDialog = panel.querySelector("#iam-brewery-dialog");
		const breweryForm = panel.querySelector("#iam-brewery-form");
		breweryDialog.addEventListener("close", async () => {
			if (breweryDialog.returnValue !== "save") {
				return;
			}
			const fd = new FormData(breweryForm);
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
				await api("/api/breweries", { method: "POST", body: JSON.stringify(body) });
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
		refreshBreweries();
	}

	async function loadSettings(panel) {
		const tanksEl = panel.querySelector("#settings-tanks");
		const multsEl = panel.querySelector("#settings-mults");
		const hygEl = panel.querySelector("#settings-hygiene");
		const beerForm = panel.querySelector("#settings-beer-price-form");
		const beerErr = panel.querySelector("#settings-beer-price-error");

		async function refresh() {
			try {
				const tanks = await api("/api/settings/tanks");
				tanksEl.innerHTML = table(
					["ID", "Name", "Capacity L"],
					(tanks || [])
						.map(
							(t) =>
								"<tr><td>" +
								t.id +
								"</td><td>" +
								esc(t.name) +
								"</td><td>" +
								t.capacity_liters +
								"</td></tr>"
						)
						.join("")
				);
				const mults = await api("/api/settings/multipliers");
				multsEl.innerHTML = table(
					["ID", "Name", "×"],
					(mults || [])
						.map(
							(m) =>
								"<tr><td>" +
								m.id +
								"</td><td>" +
								esc(m.name) +
								"</td><td>" +
								m.multiplier +
								"</td></tr>"
						)
						.join("")
				);
				const beer = await api("/api/settings/beer-price");
				beerForm.querySelector('[name="min_net_sek_per_liter"]').value =
					beer.min_net_sek_per_liter ?? 0;
				const hyg = await api("/api/settings/hygiene-routines");
				hygEl.innerHTML = table(
					["ID", "Name"],
					(hyg || [])
						.map((r) => "<tr><td>" + r.id + "</td><td>" + esc(r.name) + "</td></tr>")
						.join("")
				);
			} catch (e) {
				tanksEl.textContent = e.message;
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

		panel.addEventListener("click", async (ev) => {
			const t = ev.target;
			if (!(t instanceof HTMLElement)) {
				return;
			}
			if (t.getAttribute("data-action") === "settings-tank-new") {
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
			}
			if (t.getAttribute("data-action") === "settings-mult-new") {
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
			}
			if (t.getAttribute("data-action") === "settings-hygiene-refresh") {
				refresh();
			}
		});
		refresh();
	}
})();

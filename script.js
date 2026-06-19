const menuButton = document.querySelector("[data-menu-button]");
const navLinks = document.querySelectorAll(".topnav a");

if (menuButton) {
    menuButton.addEventListener("click", () => {
        const isOpen = document.body.classList.toggle("nav-open");
        menuButton.setAttribute("aria-expanded", String(isOpen));
    });
}

navLinks.forEach((link) => {
    link.addEventListener("click", () => {
        document.body.classList.remove("nav-open");
        if (menuButton) {
            menuButton.setAttribute("aria-expanded", "false");
        }
    });
});

document.querySelectorAll("[data-tabs]").forEach((tabsRoot) => {
    const tabs = Array.from(tabsRoot.querySelectorAll("[data-tab]"));
    const panels = Array.from(tabsRoot.querySelectorAll("[data-panel]"));

    tabs.forEach((tab) => {
        tab.addEventListener("click", () => {
            const target = tab.dataset.tab;

            tabs.forEach((item) => {
                item.classList.toggle("active", item === tab);
            });

            panels.forEach((panel) => {
                panel.classList.toggle("active", panel.dataset.panel === target);
            });
        });
    });
});

document.querySelectorAll("[data-filter-list]").forEach((filterRoot) => {
    const input = filterRoot.querySelector("[data-filter-input]");
    const items = Array.from(filterRoot.querySelectorAll("[data-filter-item]"));

    if (!input) {
        return;
    }

    input.addEventListener("input", () => {
        const query = input.value.trim().toLowerCase();

        items.forEach((item) => {
            item.hidden = query.length > 0 && !item.textContent.toLowerCase().includes(query);
        });
    });
});

const sections = Array.from(document.querySelectorAll("main section[id]"));

if ("IntersectionObserver" in window) {
    const observer = new IntersectionObserver((entries) => {
        entries.forEach((entry) => {
            if (!entry.isIntersecting) {
                return;
            }

            navLinks.forEach((link) => {
                link.classList.toggle("active", link.getAttribute("href") === `#${entry.target.id}`);
            });
        });
    }, { rootMargin: "-35% 0px -55% 0px" });

    sections.forEach((section) => observer.observe(section));
}

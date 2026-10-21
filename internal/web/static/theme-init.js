// Theme initialization - runs synchronously to prevent flash of unstyled content
(function() {
    let saved = null;
    let sidebar = null;
    try {
        saved = localStorage.getItem('onwatch-theme');
        sidebar = localStorage.getItem('onwatch-sidebar-collapsed');
    } catch (e) { /* Storage may be unavailable in private browsing. */ }
    const collapsed = sidebar === null ? window.matchMedia('(max-width: 760px)').matches : sidebar === 'true';
    document.documentElement.classList.toggle('sidebar-collapsed', collapsed);
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    const theme = saved || (prefersDark ? 'dark' : 'light');
    document.documentElement.setAttribute('data-theme', theme);
})();

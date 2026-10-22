# Dashboard chart assets

These are the existing upstream chart dependencies, served locally to avoid
blocking dashboard initialization on an external CDN. No library was added or
upgraded. Keep their versions aligned with upstream's layout template.

- Chart.js 4.4.1: https://cdn.jsdelivr.net/npm/chart.js@4.4.1/dist/chart.umd.min.js
- chartjs-adapter-date-fns 3.0.0, including its date-fns bundle:
  https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns@3.0.0/dist/chartjs-adapter-date-fns.bundle.min.js

The original file headers are preserved. The accompanying license files cover
Chart.js, the adapter, and the bundled date-fns code.

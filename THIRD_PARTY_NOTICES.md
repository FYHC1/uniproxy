# Third-party notices

- Mihomo (MetaCubeX/mihomo): GPL-3.0. UniProxy supervises an unmodified external binary. Redistributing that binary, including Docker images, requires retaining applicable notices and meeting corresponding-source obligations. Pinned source: https://github.com/MetaCubeX/mihomo/tree/v1.19.32 . Upstream LICENSE: https://github.com/MetaCubeX/mihomo/blob/v1.19.32/LICENSE . UniProxy’s integrated online installation verifies official release SHA256 assets against built-in fixed digests before extraction. Current Docker images do not prebundle Mihomo; explicit installation writes it into the persistent data volume. This change does not waive obligations when redistributing an image or volume that contains Mihomo.
- React and Vite: MIT.
- Radix UI and Phosphor icons: MIT.
- modernc.org/sqlite: see upstream distribution/license notices; SQLite itself is public domain. Transitive modules have their own licenses.
- gopkg.in/yaml.v3: MIT / Apache-2.0 (upstream dual license).

This notice is an inventory, not a substitute for reviewing the exact dependencies and distribution artifacts. UniProxy project license is intentionally left for the project owner to choose.

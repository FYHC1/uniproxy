# UniProxy frontend verification

Run from the repository root after the lead installs the declared dependencies:

```sh
node node_modules/typescript/bin/tsc --noEmit --noUnusedLocals --noUnusedParameters
node --test src/web/tests/utils.test.mjs
node src/web/tests/verify.mjs
```

The unit suite uses Node 22.18+ / 24's TypeScript stripping and has no additional dependencies.
The browser suite uses the existing Vite, React plugin and Playwright dependencies. It uses installed Playwright Chromium, or local Chrome when the Playwright browser is absent.

The browser test builds the production app **in memory** with Vite `write:false` and intercepts a test origin with Playwright routes. It starts **no server** and writes only screenshots under this test directory's ignored `artifacts/` directory. Fixtures exist only in this regression script and are not imported by the app or emitted into the app bundle. Screenshots containing fixtures are **not evidence of working proxy connectivity**.

## Covered

- Empty bootstrap and unavailable-core warning; never claimed running.
- 401 authentication, sessionStorage-only token, no URL/localStorage secrets; expired-token lock.
- Five-second bootstrap polling, offline banner and retention of last confirmed data.
- Text import preview without mutation, confirmation, new group names, local-file and subscription preview modes.
- Accessible keyboard browse and synthetic drag/drop: protected drag detection, nested hover feedback, navigation prevention inside/outside the zone, directory (entry and handle APIs), multi-file, empty/read-error and exact 5 MiB boundary handling.
- Deferred file reads and preview success/error responses: newer selection, mode change, changed content/groups and dialog close invalidate obsolete work; import-in-progress blocks replacement.
- Relay metadata in preview/table/details without raw configs; missing/disabled relay warnings and truthful totalErrors/errorsTruncated displays with at most 128 details and conservative legacy fallbacks.
- Node search and enable/disable batch payloads; independent include-any/all, exclude-any precedence, only-ungrouped, combined predicates, summary badges, full/group/page resets and preserved hidden selections.
- Canonical pool membership any/all, exclusions, disabled and UDP filters; scoped chooser default checks, uncheck-to-exclusion, explicit range pruning, legacy exclusion retention and deleted-ref cleanup.
- Bulk selection covers all filtered results (including >100), preserves selections outside search, and supports mixed/disabled states.
- Combined pool+listener form payload with authentication, pool cards for multiple IPv4/IPv6 endpoints and truthful dirty/running status, actual test-job request payloads, safe pool-only removal/reinclude, task record loss after service restart and search remaining clearable after member count shrinks.
- Group create, all four pool strategy controls, in-user hash option and manual includes.
- Listener same-username password omission, new username requires password; HTTP disables UDP; public UDP denied.
- Jobs cancellation, confirmed force-refresh, subscription edits.
- Settings save, sensitive export confirmation cancellation, backup restore preview and cancellation.
- 375 / 768 / 1024 viewport document overflow checks, viewport-contained mobile group panel, keyboard opening/closing, modal Escape, and no uncaught browser errors.

The node filter's only-ungrouped shortcut clears and disables inclusion choices (including any/all mode), while preserving independent exclusions. Both reset controls reset the page and all group-filter state; the global reset also clears search/protocol/status/UDP. Filtering never clears the selected IDs silently.

The browser harness imports [relay regressions](<relay-regressions.mjs>) and [pool regressions](<pool-regressions.mjs>) only in the test runner; the product bundle does not import them. Test-generated screenshots are synthetic UI evidence only. The pure utility suite currently contains 36 tests (including four installation schema/prompt/progress helpers); compact pool metrics never convert unknown/failed latency into zero. Pool fixtures cover six-row pagination, width/height bounds, endpoint disclosure and revision-mismatched listener state. Real Go API browser scenarios are maintained separately under [browser tests](<../../../tests/browser/>). The fixture runner prefers the bundled Playwright browser, then PLAYWRIGHT_CHROME_PATH, then a detected system Chrome/Edge under Program Files or the usual Linux paths. Resize assertions wait up to 2.5 seconds for media-query layout to settle and fail on persistent horizontal overflow; an intermediate frame during viewport change is not a layout result.

Additional real-API browser specs cover direct group node management (20-row pages, >100 add choices, current-scope stale selection cleanup, source warnings, global disable confirmation), compact pool layouts and safe subscription failure display. Some group UI state projections are explicitly synthetic; POST /tests still targets the missing-core API and must fail rather than fake a measurement.

## Built-in core installation regressions

The [installation fixture suite](<core-install-regressions.mjs>) is invoked by the existing [runner](<verify.mjs>). Every browser request is intercepted at a synthetic origin; unexpected external requests fail. It covers:

- Prompt only after authenticated bootstrap, once per tab/path, dismissal across polling/reloads and page navigation, and no automatic modal for an already-active job.
- Runtime and settings-tab entries, Radix focus trap/Escape/focus return, long installation paths and 375/390/768px layouts, with reduced motion.
- Shared install/inspect request lock across duplicate surfaces, dialog dismissal during a pending request/download, persistent global progress and errors.
- Real downloaded-byte percentage only during a known-size transfer; indeterminate metadata/unknown-size/verify/extract/install/inspect stages do not fabricate percentages.
- Success requires explicit runtime Apply; existing compatible cores cannot be reinstalled, incompatible destinations are not overwritten, unsupported/empty-path reasons remain actionable through explicit inspect.
- Failure/retry only when allowed, 409 without automatic POST retry, offline retained state, install/inspect 401 auth reset, obsolete 202/401 responses after authentication changes, and stale pre-install bootstrap responses.
- Required installation bootstrap validation and explicit manual-inspection recovery without downloading or applying.

The [pure helper tests](<core-install.test.mjs>) are imported by the normal [unit entry](<utils.test.mjs>); TypeScript compilation stays in memory. Synthetic screenshots remain under the ignored artifacts directory and are **not** evidence of a downloaded binary or running proxy.

Real-API [installation browser tests](<../../../tests/browser/core-install.spec.ts>) cover a real missing-core bootstrap/prompt and explicit GET status/POST inspect without revision change or proxy start. Other feature tests use [gotoWorkspace](<../../../tests/browser/fixtures.ts>) to dismiss the real startup prompt without faking availability or weakening their existing state assertions. The shared browser fixture aborts and fails unexpected install requests; the isolated testserver adds a server-side offline guard. Network installation is never part of browser tests.

## Integration boundaries

The test is an API-contract / browser regression, not a live Go / SQLite / Mihomo test. Actual listener binds, authenticated proxy traffic, latency/speed results, remote subscriptions and restore persistence must be validated with the lead's real backend and core integration suite.

Implementation consumes canonical types from [shared types](<../../shared/types.ts>) and follows the [implementation contract](<../../../docs/implementation-contract.md>). Root dependencies and build configuration remain lead-owned; the installation UI task also maintains the required public TypeScript installation contract.

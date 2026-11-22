# LaborControl R11 — реестр файлов для пофайлового аудита, 23.11.2026

**Ветка:** `fix/r11-ux-and-employee-20261122`. Состав получен из текущих каталогов GitHub Contents API (10 каталогов). Список покрывает код, скрипты, CI и архив; документы `docs/` хранят спецификации и отчёты и здесь не считаются исполняемым кодом.

**Правила:** `REVIEWED_STATIC` — текст изучен, но это не гарантия отсутствия дефектов; `PARTIAL` — просмотрены отдельные функции и точки входа; `PENDING` — полный разбор ещё впереди; `ARCHIVE_NOT_CURRENT` — исторический код, изолирован от текущей сборки (не путать с production). Тесты CI не доказывают полный построчный аудит.

**Сводка:** REVIEWED_STATIC — 9, PARTIAL — 18, PENDING — 36, ARCHIVE_NOT_CURRENT — 20. Всего 83 файлов.

## port/controller

| Путь | Размер, байт | Статус |
|---|---:|---|
| `port/controller/employee_guide_v7.html` | 908712 | PENDING |
| `port/controller/frontend.html` | 341197 | PENDING |
| `port/controller/go.mod` | 41 | PENDING |
| `port/controller/main.go` | 26038 | REVIEWED_STATIC |
| `port/controller/platform_linux.go` | 2679 | PARTIAL |
| `port/controller/platform_linux_test.go` | 1579 | PENDING |
| `port/controller/platform_windows.go` | 2700 | PENDING |
| `port/controller/r11_api_contract_test.go` | 1439 | PENDING |
| `port/controller/r11_atomic_file_safety_test.go` | 2078 | PARTIAL |
| `port/controller/r11_core.go` | 77461 | REVIEWED_STATIC |
| `port/controller/r11_core_test.go` | 14278 | PENDING |
| `port/controller/r11_data_safety_test.go` | 7346 | PARTIAL |
| `port/controller/r11_deep_integrity_test.go` | 9901 | PENDING |
| `port/controller/r11_desktop_mode_test.go` | 1419 | PENDING |
| `port/controller/r11_embedded_tab2_test.go` | 1344 | PENDING |
| `port/controller/r11_employee_batch_test.go` | 6558 | PARTIAL |
| `port/controller/r11_employee_help_test.go` | 2357 | PARTIAL |
| `port/controller/r11_http_security.go` | 2103 | REVIEWED_STATIC |
| `port/controller/r11_http_security_test.go` | 3266 | REVIEWED_STATIC |
| `port/controller/r11_input_validation.go` | 6023 | REVIEWED_STATIC |
| `port/controller/r11_original.html` | 346343 | PENDING |
| `port/controller/r11_point_template.go` | 4706 | PARTIAL |
| `port/controller/r11_point_template_test.go` | 1820 | PARTIAL |
| `port/controller/r11_revision_restore_test.go` | 6453 | PARTIAL |
| `port/controller/r11_russian_ui_test.go` | 2654 | PENDING |
| `port/controller/r11_schema.go` | 8383 | PENDING |
| `port/controller/r11_schema_test.go` | 5180 | PENDING |
| `port/controller/r11_stage_validation.go` | 2539 | REVIEWED_STATIC |
| `port/controller/r11_stage_validation_test.go` | 3409 | REVIEWED_STATIC |
| `port/controller/r11_state.go` | 15685 | REVIEWED_STATIC |
| `port/controller/r11_state.html` | 15504 | PENDING |
| `port/controller/r11_state_test.go` | 10870 | PENDING |
| `port/controller/r11_tab2_plan_integration_test.go` | 1472 | PENDING |
| `port/controller/r11_ui.go` | 3216 | PARTIAL |
| `port/controller/r11_ux_regression_test.go` | 4814 | PENDING |
| `port/controller/r11_xlsx.go` | 4405 | PARTIAL |
| `port/controller/r11_xlsx_test.go` | 5921 | PENDING |
| `port/controller/startup_r9_test.go` | 845 | PENDING |
| `port/controller/tab2.go` | 476 | PENDING |
| `port/controller/tab2_import.go` | 28500 | PARTIAL |
| `port/controller/tab2_integration_test.go` | 1692 | PENDING |
| `port/controller/tab2_logic_test.go` | 4742 | PENDING |
| `port/controller/tab2_state.go` | 19064 | PARTIAL |
| `port/controller/tab2_types.go` | 9939 | PARTIAL |
| `port/controller/xlsx.go` | 9949 | PARTIAL |

## port/controller/tests

| Путь | Размер, байт | Статус |
|---|---:|---|
| `port/controller/tests/eslint.config.mjs` | 1546 | PENDING |
| `port/controller/tests/extract_ui_scripts.cjs` | 983 | PENDING |
| `port/controller/tests/r11_file_picker_smoke.cjs` | 5082 | PARTIAL |
| `port/controller/tests/r11_ui_smoke.cjs` | 13868 | PARTIAL |
| `port/controller/tests/smoke_linux.sh` | 3110 | PENDING |

## port/desktop

| Путь | Размер, байт | Статус |
|---|---:|---|
| `port/desktop/README_RU.md` | 4219 | PENDING |
| `port/desktop/build.sh` | 1099 | PARTIAL |
| `port/desktop/install-user.sh` | 1574 | REVIEWED_STATIC |
| `port/desktop/laborcontrol.svg` | 563 | PENDING |
| `port/desktop/main.c` | 18278 | PARTIAL |

## port/desktop/tests

| Путь | Размер, байт | Статус |
|---|---:|---|
| `port/desktop/tests/webkit_smoke.py` | 4554 | PENDING |

## tools

| Путь | Размер, байт | Статус |
|---|---:|---|
| `tools/export_r11_schema.ps1` | 3581 | PENDING |

## .github/workflows

| Путь | Размер, байт | Статус |
|---|---:|---|
| `.github/workflows/alt-linux-controller-tests.yml` | 3175 | PENDING |
| `.github/workflows/alt-linux-runner-smoke.yml` | 1204 | PENDING |
| `.github/workflows/linux-controller.yml` | 2633 | PENDING |
| `.github/workflows/linux-r11-desktop.yml` | 4654 | PENDING |
| `.github/workflows/r11-js-static-audit.yml` | 1273 | PENDING |
| `.github/workflows/r11-schema-exporter-check.yml` | 2039 | PENDING |

## archive/browser-shim

| Путь | Размер, байт | Статус |
|---|---:|---|
| `archive/browser-shim/go.mod` | 41 | ARCHIVE_NOT_CURRENT |
| `archive/browser-shim/main.go` | 28 | ARCHIVE_NOT_CURRENT |

## archive/build-scripts

| Путь | Размер, байт | Статус |
|---|---:|---|
| `archive/build-scripts/build_windows.bat` | 520 | ARCHIVE_NOT_CURRENT |
| `archive/build-scripts/build_windows.ps1` | 575 | ARCHIVE_NOT_CURRENT |
| `archive/build-scripts/make_linux.ps1` | 1833 | ARCHIVE_NOT_CURRENT |
| `archive/build-scripts/test_source.bat` | 114 | ARCHIVE_NOT_CURRENT |

## archive/controller-r9

| Путь | Размер, байт | Статус |
|---|---:|---|
| `archive/controller-r9/frontend.html` | 329605 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/go.mod` | 41 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/main.go` | 24270 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/platform_windows.go` | 1282 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/startup_r9_test.go` | 825 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/tab2.go` | 476 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/tab2_import.go` | 28500 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/tab2_integration_test.go` | 1692 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/tab2_logic_test.go` | 4742 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/tab2_state.go` | 19064 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/tab2_types.go` | 9878 | ARCHIVE_NOT_CURRENT |
| `archive/controller-r9/xlsx.go` | 9949 | ARCHIVE_NOT_CURRENT |

## archive/legacy-v0.3

| Путь | Размер, байт | Статус |
|---|---:|---|
| `archive/legacy-v0.3/go.mod` | 28 | ARCHIVE_NOT_CURRENT |
| `archive/legacy-v0.3/main.go` | 23592 | ARCHIVE_NOT_CURRENT |

## Порядок дальнейшей проверки

1. Доисследовать `tab2_import.go`, `tab2_state.go`, `tab2_types.go` и Excel-парсеры; регрессии целостности двух хранилищ и ZIP.
2. Детально разобрать `r11_original.html`, `frontend.html`, GTK и десктопный WebKit, сравнить интерфейс и API.
3. Верифицировать все Go/JS/GTK тесты на активном коде, проверить соответствие требованиями Windows R11.
4. Только затем проверить архивы на фактически переносимые функции (архивные исходники не входят в активную сборку).

Промежуточные результаты, подтверждённый фикс AG-AUDIT-001 и CI: `R11_LINE_BY_LINE_AUDIT_20261123.md`.

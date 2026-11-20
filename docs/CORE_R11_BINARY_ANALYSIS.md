# Анализ Windows Core R11 и план восстановления

**Дата:** 20.11.2026  
**Ветка:** `feature/r11-core-recovery-20261120`  
**Цель:** использовать сохранённый Windows Core R11 как проверяемый эталон для восстановления полноценной Linux-версии, не подменяя его демонстрационным экраном или старой v0.3.

## 1. Проверенный оригинальный бинарник

Файл `LaborControl_Core_R11.exe` найден в сохранённой библиотеке проекта (не в Git-репозитории) и исследован статически.

- Размер: 6 716 928 байт.
- Формат: PE32+ / Windows x86-64.
- Go build info: `go1.23.2`, `GOOS=windows`, `GOARCH=amd64`, `CGO_ENABLED=0`.
- SHA-256: `f0fe314aa0c6ddbebf8ba7aca6d6a0e57826325a3d2d6246cd6016baa6b541c4`.
- Сумма совпадает с `docs/reference/SHA256.txt`.

**Ограничение:** это Windows-бинарник, он не является исходным кодом и не запускается нативно на ALT Linux. Обычная Go-таблица символов удалена (`go tool nm` сообщает `no symbols`). Исходники Core v0.9.11 в комплекте по-прежнему не обнаружены.

При этом в бинарнике сохранилась Go pclntab. Её статическое чтение позволило восстановить список из 6 318 записей функций; среди них функции приложения `main.*`. Это имена функций, а не их исходный код и не доказательство корректности алгоритмов.

## 2. Что удалось подтвердить в Core R11

В бинарнике присутствуют обработчики и методы приложения, в частности:

- состояние и хранилище: `apiState`, `Store.Load`, `Store.saveLocked`, `Store.migrateLocked`, `defaultDB`, `preMigrationSafetyCopy`;
- сотрудники и дни: `apiEmployeeSave`, `apiEmployeeDelete`, `apiDaySave`, `apiDayDelete`;
- календарь: `apiCalendarInit`, `apiCalendarClear`, `apiCalendarDaySave`, `apiCalendarImportConfirm`;
- виды графиков и полёты: `apiScheduleTypeSave`, `apiScheduleTypeDelete`, `apiFlightSave`, `apiFlightDelete`, `apiFlightBatch`, `apiFlightImportPreview`, `apiFlightImportConfirm`;
- редакции и версии: `apiVersionCreate`, `apiVersionRestore`, `apiScheduleChange`, `apiRevisionCreate`, `apiRevisionGet`, `apiRevisionActivate`, `Store.activeRevisionLocked`, `Store.revisionSnapshotLocked`;
- примечания, цвета и рейсы сотрудников: `apiNoteSave`, `apiNoteDelete`, `apiNoteReminderDelete`, `apiNoteSplit`, `apiColorSave`, `apiColorDelete`, `apiEmployeeFlightImportConfirm`, `apiEmployeeFlightDelete`;
- импорт и обслуживание: `apiImportEvents`, `apiImportFact`, `apiBackup`, `apiShutdown`.

В строках бинарника также обнаружены имена структурных полей/коллекций: `flight_dates`, `flight_routes`, `schedule_types`, `schedule_type`, `graph_revisions`, `schedule_changes`, `employee_flights`, `manual_colors`, `import_history`, `schema_version`, `dismissal_date` и другие.

Это подтверждает, что Core R11 содержит полноценный backend для сотрудников, АвтоГрафа, календаря, полётов, редакций и других функций. Это не только старый CRUD из v0.3.

## 3. Что уже есть в исходниках репозитория

- `port/controller/frontend.html` — сохранённый интерфейс (HTML/CSS/JS); в нём обнаружено 443 объявления именованных JS-функций, включая функции АвтоГрафа, построения графика по полётам, циклов, проверки графика, редакций и перехода года.
- Во фронтенде обнаружено 39 API-маршрутов. Среди них `/api/state`, `/api/day/save`, `/api/day/quick-save`, `/api/autograf/flight/*`, `/api/autograf/schedule-type/*`, `/api/autograf/revision/*`, `/api/autograf/version/*`, а также API сотрудников, событий, календаря и примечаний.
- `port/controller/main.go` — внешний контроллер R9: запускает Core, проксирует `/api/*`, реализует bulk stage/apply и подключает обработчики Табеля 2.0.
- `port/controller/tab2_*.go` — доступные исходники внешнего контроллера и Табеля 2.0.
- `archive/legacy-v0.3/main.go` — исторический backend v0.3 с базовыми обработчиками сотрудников, дней, событий и РВД. Его нельзя считать эквивалентом Core R11.
- `docs/reference/CHANGELOG_v1_0_FULL_PREVIEW.txt`, `README_RU.txt`, `TAB2_RULES.md` — документируют часть изменений и правил R11.

## 4. Вывод и безопасная стратегия

Не нужно продолжать развивать отдельный экран `port/schedules-preview` как замену R11. Он остаётся только smoke/demo и не сливается с основным портом до отдельного решения.

Следующий инженерный этап — получить **обезличенную структурную схему реального ответа R11 `/api/state`** с Windows-экземпляра. Для этого подготовлен `tools/export_r11_schema.ps1`. Скрипт обращается только к локальному адресу `127.0.0.1:8779` и сохраняет исключительно названия полей, типы и количество элементов массивов. Он не сохраняет значения полей, табельные номера, ФИО, даты, комментарии, содержимое базы или путь к базе и ничего не отправляет в интернет.

После получения схемы:
1. Зафиксировать точные модели и JSON-контракт R11.
2. Сопоставить поля с `legacy-v0.3`, R9 controller и документами R11; отдельно пометить поля/поведение, отсутствующие в старой версии.
3. Выделить Core-совместимый backend в Linux-проект, начиная с чтения состояния и CRUD графика на копиях/синтетических данных.
4. Добавить регрессионные тесты для правил R11, особенно привязки вида графика к конкретному циклу полётов и сменности.
5. Сравнивать одинаковые сценарии на Windows R11 и ALT Linux; не заявлять функциональное совпадение до сравнения.

**Не загружать в GitHub:** Core EXE, рабочие базы, реальные выгрузки, персональные данные и несанкционированные копии. Текущий отчёт содержит только статические метаданные бинарника и имена функций/полей.

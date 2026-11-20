# LaborControl Core R11 — анализ схемы API

**Дата получения:** 20.11.2026  
**Источник:** обезличенный файл `LaborControl_R11_schema.json`, сформированный на Windows из локального `GET http://127.0.0.1:8779/api/state`.  
**Назначение:** зафиксировать фактическую форму ответа работающего Windows Core R11 для восстановления совместимого Linux backend.

## 1. Что именно получено

Верхний уровень ответа содержит поля:

- `data`
- `data_path`
- `discrepancies`
- `version`

`data` — объект с 18 полями. В отчёт попали только названия полей, типы, количества элементов массивов и имена/типы полей первых двух объектов непустых массивов. Значения сотрудников, даты, комментарии, пути к базе и содержимое записей не экспортировались.

Снимок получен **20.11.2026 15:01:24 UTC**.

## 2. Коллекции и объёмы текущего тестового снимка

| Поле | Тип | Количество | Поля объекта (объединение первых двух образцов, где отличается) |
|---|---|---:|---|
| `calendar` | array | 1 096 | `date:string`, `day_type:string`, `norm_hours:Int32`, `note:string`, `source:string` |
| `days` | array | 1 532 | `comment:string`, `date:string`, `employee_id:Int32`, `fact:string`, `fact_hours:Int32`, `id:Int32`, `plan:string`, `plan_hours:Int32`, `shift:string`, `state:string`, `updated_at:string` |
| `employee_flights` | array | 0 | форма элемента не получена: массив пуст |
| `employees` | array | 6 | `collection_point:string`, `fire_date:string`, `full_name:string`, `hire_date:string`, `id:Int32`, `is_quarter:boolean`, `main_schedule_ready:boolean`, `notes:string`, `position:string`, `probation:boolean`, `probation_end:string`, `schedule_type:string`, `tab_no:string`, `unit:string` |
| `events` | array | 54 | `created_at:string`, `date_from:string`, `date_to:string`, `employee_id:Int32`, `event_type:string`, `id:Int32`, `note:string`, `source:string`, `source_code:string` |
| `flight_dates` | array | 62 | `collection_point:string`, `date:string`, `id:Int32`, `note:string`, `source:string` |
| `flight_routes` | array | 8 | `active:boolean`, `collection_point:string`, `id:Int32`, `name:string` |
| `graph_revisions` | array | 13 | `created_at:string`, `effective_date:string`, `employee_id:Int32`, `fixed_hours:Int32`, `id:Int32`, `label:string`, `number:Int32`, `parent_id:Int32`, `reason:string`, `schedule_type:string`, `status:string`, `updated_at:string`; во втором образце дополнительно присутствуют `activated_at:string` и `control_date:string` |
| `graph_versions` | array | 83 | `created_at:string`, `days:array`, `employee_id:Int32`, `id:Int32`, `label:string`, `reason:string`, `year:Int32` |
| `import_history` | array | 7 | `added:Int32`, `created_at:string`, `file_name:string`, `id:Int32`, `moved:Int32`, `skipped:Int32`, `type:string`, `updated:Int32` |
| `journal` | array | 11 420 | `details:string`, `operation:string`, `time:string` |
| `manual_colors` | array | 9 | `color:string`, `date:string`, `employee_id:Int32`, `id:Int32`, `updated_at:string` |
| `notes` | array | 7 | `created_at:string`, `date_from:string`, `date_to:string`, `employee_id:Int32`, `id:Int32`, `reminder:string`, `text:string`, `updated_at:string` |
| `rvd` | array | 50 | `created_at:string`, `date_from:string`, `date_to:string`, `employee_id:Int32`, `id:Int32`, `note:string`, `rest_date:string`, `type:Int32` |
| `schedule_changes` | array | 0 | форма элемента не получена: массив пуст |
| `schedule_types` | array | 6 | `active:boolean`, `collection_point:string`, `id:Int32`, `name:string`, `note:string`, `pair_name:string`, `phase:string` |

Дополнительные скалярные поля:

- `next_id: Int32`
- `schema_version: Int32`

Поле ответа `discrepancies` — массив, в текущем снимке 6 элементов. Форма его элементов в экспорт не включена.

## 3. Ограничения полученной схемы

1. Количества — это только состояние тестовой базы на момент запроса. В ней 6 сотрудников; это **не ограничение R11** и не нагрузочный тест для целевых 450 сотрудников.
2. Для пустых `employee_flights` и `schedule_changes` типы полей элементов неизвестны.
3. Вложенный массив `graph_versions[].days` обнаружен, но его внутренняя структура этим экспортом не раскрыта.
4. Для `graph_revisions` первые два образца имеют различающиеся поля: `activated_at` и `control_date` есть во втором. Реализация должна учитывать необязательные/версионные поля, а не считать схему каждого элемента абсолютно одинаковой.
5. Экспорт не описывает HTTP-методы, коды ошибок, правила валидации, последовательность изменений и бизнес-алгоритмы. Их нужно восстанавливать отдельными безопасными сценариями на Windows R11.

## 4. Сопоставление с текущим Linux-портом

Проверена ветка `feature/r11-core-recovery-20261120`.

- `port/controller/main.go` объявляет `controllerVersion = "v1.0-TIMESHEET-PREVIEW-R9"`.
- В Windows-слое контроллера указано имя `LaborControl_Core_R9.exe`; в Linux-слое `platformCoreName()` возвращает `LaborControl_Core`.
- Маршрут `/api/*` в контроллере является reverse proxy к дочернему Core. Сам по себе контроллер не реализует backend R11.
- Во фронтенде найдены обращения к 43 API-путям, включая АвтоГраф, полёты, редакции/версии, календарь, сотрудников, события, РВД и Табель 2.0. Наличие вызова во frontend не доказывает, что соответствующая реализация Core уже перенесена.
- Исходный код полного Windows Core R11 в репозитории отсутствует. Текущий Linux bootstrap/preview нельзя считать полноценным переносом R11.
- Код Табеля 2.0 и часть контроллера уже существуют отдельно; это не заменяет недостающий Core R11.

**Вывод:** схема подтверждает реальный контракт состояния R11 и позволяет начать строгую модель данных. Но она не доказывает функциональную эквивалентность и не раскрывает логику АвтоГрафа/полётов.

## 5. Следующие инженерные шаги

1. Зафиксировать Go-модели JSON для всех 18 полей `data`, включая optional fields и неизвестные поля, чтобы не терять данные при чтении/сохранении.
2. Добавить тесты round-trip JSON: чтение схемоподобного fixture → сериализация → повторное чтение, с проверкой сохранения всех коллекций и полей.
3. Отдельно получить структурные образцы для `employee_flights`, `schedule_changes`, `discrepancies` и вложенного `graph_versions[].days` (без значений персональных данных).
4. Реализовать на Linux сначала совместимый `GET /api/state` и безопасное локальное хранилище/резервное копирование.
5. Затем восстанавливать операции Core группами: сотрудники/дни/календарь; типы графиков и полёты; редакции/версии; события/РВД/заметки/цвета/журнал.
6. Для каждой операции сравнивать Windows R11 и Linux на одинаковых синтетических сценариях; фиксировать результат в `docs/TEST_MATRIX_R11.md`.
7. Провести отдельный нагрузочный тест на синтетической базе порядка 450 сотрудников и многолетней истории. Текущий снимок не является таким тестом.

## 6. Безопасность и данные

В Git добавлен только этот структурный анализ. Не добавлять в репозиторий рабочие базы, реальные выгрузки ИСАТ/SAP, значения ФИО/табельных номеров, комментарии или содержимое журнала. Для тестов использовать синтетические данные и отдельные копии.

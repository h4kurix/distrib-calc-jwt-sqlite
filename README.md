# Распределённый вычислитель арифметических выражений

Сервис принимает арифметическое выражение, разбивает его на атомарные задачи
и распределяет их между агентами. Оркестратор хранит состояние в SQLite,
агенты получают задачи по внутреннему API и возвращают результаты.

## Возможности

- операции `+`, `-`, `*`, `/`, скобки и унарный минус;
- JWT-аутентификация пользователей и отдельных токенов агентов;
- параллельная обработка задач агентами;
- SQLite (WAL) как хранилище выражений, задач и пользователей;
- Docker Compose для запуска.

## Установка и запуск

```bash
git clone https://github.com/h4kurix/distrib-calc-jwt-sqlite
cd distrib-calc-jwt-sqlite
cp .env.example .env
```

Затем задайте в `.env` свои значения `JWT_SECRET` и `AGENT_BOOTSTRAP_KEY`.
Это обязательно: сервис откажется стартовать с заглушкой из примера.

```bash
docker compose up --build
```

Сервис доступен на `http://localhost:8080`.

### Масштабирование агентов

Поле `deploy.replicas` в Compose без Swarm не работает. Число агентов задаётся
при запуске:

```bash
docker compose up --build --scale agent=3
```

Переменная `COMPUTING_POWER` задаёт число воркеров внутри одного контейнера
агента. Оба механизма можно комбинировать.

## Конфигурация

Все параметры читаются из окружения. Основные:

| Переменная | Назначение | По умолчанию |
|---|---|---|
| `JWT_SECRET` | ключ подписи JWT, обязателен | — |
| `AGENT_BOOTSTRAP_KEY` | ключ выдачи токенов агентов, обязателен | — |
| `ENV`, `LOG_LEVEL` | окружение и уровень логов | `dev`, `info` |
| `PORT`, `ORCHESTRATOR_PORT` | порты оркестратора и адрес агента | `8080` |
| `DB_PATH` | путь к SQLite | `data/calculator.db` |
| `COMPUTING_POWER` | воркеров в контейнере агента | `3` |
| `MAX_EXPRESSION_LEN`, `MAX_TASKS_PER_EXPR` | лимиты сложности выражения | `256` |
| `MAX_OPERATION_TIME_MS` | потолок времени операции | `30000` |
| `AGENT_TOKEN_TTL` | срок жизни токена агента | `24h` |
| `USER_TOKEN_TTL` | срок жизни пользовательского токена | `24h` |
| `TASK_POLL_INTERVAL` | интервал опроса задач агентом | `500ms` |
| `STATIC_DIR` | каталог статики оркестратора | `./static` |
| `TIME_ADDITION_MS`, `TIME_SUBTRACTION_MS`, `TIME_MULTIPLICATIONS_MS`, `TIME_DIVISIONS_MS` | время операции по типу | `100`, `100`, `200`, `300` |

Публичные роуты `register` и `login` ограничены: 20 запросов с адреса в минуту. При превышении сервер отвечает `429` с JSON.

## API

### Регистрация

```bash
curl -X POST http://localhost:8080/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"password123"}'
```

### Вход

```bash
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"password123"}'
```

Ответ: `{"token":"..."}`.

### Вычисление

```bash
curl -X POST http://localhost:8080/api/v1/calculate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"expression":"2+3*4"}'
```

Ответ: `{"id":"expr-..."}`.

### Статус выражения

```bash
curl http://localhost:8080/api/v1/expressions/expr-... \
  -H "Authorization: Bearer <token>"
```

Возможные статусы: `pending`, `in_progress`, `completed`, `error`.
Пользователь видит только свои выражения.

### Список выражений

```bash
curl http://localhost:8080/api/v1/expressions \
  -H "Authorization: Bearer <token>"
```

## Тесты

```bash
go test ./... -race
go vet ./...
go build ./...
```

## Архитектура

- `cmd/orchestrator` — HTTP API, выдача задач, финализация выражений.
- `cmd/agent` — воркеры, получение задач и отправка результатов.
- `internal/calculator` — валидация, токенизация, дерево выражения, агрегация.
- `internal/store` — доступ к SQLite, атомарный захват задач.
- `internal/handler` — HTTP-слой, проверка JWT, владельческая проверка.
- `pkg/config` — конфигурация из окружения, `pkg/database` — SQLite, `pkg/logger` — логи.

Задача захватывается в транзакции (`status = claimed`), поэтому два агента
не получат одну и ту же задачу. Финализация выражения выполняется в обработчике
результата, без фонового тикера — это убирает гонку с перезаписью результата.
Зависшие задачи (`claimed` дольше двух минут) возвращаются в очередь.

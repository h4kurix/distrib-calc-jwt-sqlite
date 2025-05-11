# Распределённый вычислитель арифметических выражений

## Особенности
- 🧮 Поддержка операций: +, -, *, /, скобки ()
- 🔒 JWT-аутентификация и авторизация
- ⚙️ Параллельная обработка задач
- 📈 Автомасштабирование вычислительных агентов
- 📊 Мониторинг статуса вычислений
- 🐳 Docker-контейнеризация

## Установка и запуск
```bash
git clone https://github.com/h4kurix/distrib-calc-jwt-sqlite
cd distrib-calc-jwt-sqlite
docker-compose up --build
```
Система будет доступна на `http://localhost:8080`

## Запуск тестов
```bash
# Запуск тестов с подробным выводом
go test ./... -v
```

## Работа с API

### 1. Регистрация пользователя
```bash
curl -X POST http://localhost:8080/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"test"}'
```

### 2. Авторизация
```sh
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"test"}'

```
```json
{
  {"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NDcwMDQzMTIsInN1YiI6InVzZXItMTc0NjkxNzkxMjk1NTkyNzQ3MiJ9.43cByuZk9TkyewuqFRdKpnNv0jkBY9fsydePCasIcuA"}
}
```

### 3. Отправка выражения для вычисления
```bash
curl -X POST http://localhost:8080/api/v1/calculate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NDcwMDQzMTIsInN1YiI6InVzZXItMTc0NjkxNzkxMjk1NTkyNzQ3MiJ9.43cByuZk9TkyewuqFRdKpnNv0jkBY9fsydePCasIcuA" \
  -d '{"expression": "2+2"}'
  ```

```json
{"id":"expr-1746917983695779570"}
```

### 4. Проверка статуса выражения
```bash
curl -X GET http://localhost:8080/api/v1/expressions/expr-1746917983695779570 \
-H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NDcwMDQzMTIsInN1YiI6InVzZXItMTc0NjkxNzkxMjk1NTkyNzQ3MiJ9.43cByuZk9TkyewuqFRdKpnNv0jkBY9fsydePCasIcuA"
```

```json
{"expression":{"id":"expr-1746917983695779570","status":"completed","result":4}}
```

### 5. Получение списка выражений
```bash
curl --location 'localhost:8080/api/v1/expressions' \
-H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NDcwMDQxMzIsInN1YiI6InVzZXItMTc0NjkxNzcyODUwNjE5NTY3MCJ9.ZTbOTSbSs5anNBCHtn_LV4Z_nU99jIn8DG5e0dUh8gM"
```

```json
{
  "expressions": [
    {
      "id": "expr-1746918346191291752",
      "status": "completed",
      "result": -244395958067760
    },
    {
      "id": "expr-1746918287472350858",
      "status": "completed",
      "result": 1.275906745801213e+22
    },
    {
      "id": "expr-1746917738341600427",
      "status": "completed",
      "result": 6
    }
  ]
}
```

## Внутреннее API (для агентов)

### 1. Получение токина агента (доступ из локальной сети)
```bash
curl -X GET http://localhost:8080/internal/agent/token
```

```json
{"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NDcwMDYwNjgsInJvbGUiOiJhZ2VudCJ9._m4_w7x53pw-RN4eaDxIfEY0nxTp4V-1g9syTGBCCBU"}
```



### 2. Получение задачи для выполнения

```bash
curl --location 'localhost:8080/internal/task'
```

```json
{
  "task": {
    "id": "task-9da9894f-aaba-4642-a80d-6e7eca30ab8f",
    "expression_id": "expr-1746919730164248661",
    "arg1": "493934",
    "arg2": "49433333349",
    "operation": "-",
    "operation_time": 100,
    "user_id": ""
  }
}
```
### 2. Получение результата выполнения задачи
```bash
curl -X GET http://localhost:8080/internal/task/result/task-4dcbb147-c29b-4b66-8d79-00f786c43e59 \
-H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NDcwMDYwNjgsInJvbGUiOiJhZ2VudCJ9._m4_w7x53pw-RN4eaDxIfEY0nxTp4V-1g9syTGBCCBU"
```

```json
{"result":-4.444444688895057e+21}
```
## Архитектура системы

### Компоненты
1. **Оркестратор**:
    - Приём выражений от пользователей
    - Разбор выражений на атомарные задачи
    - Распределение задач между агентами
    - Сбор результатов
    - Управление состоянием системы
2. **Агенты**:
    - Выполнение отдельных арифметических операций
    - Автомасштабирование по требованию
    - Повторное выполнение при ошибках

### Схема взаимодействия
```mermaid
sequenceDiagram
    participant User
    participant Orchestrator
    participant Agent

    User->>Orchestrator: POST /calculate
    Orchestrator->>Orchestrator: Разбор выражения
    loop Для каждой операции
        Orchestrator->>Agent: Назначение задачи
        Agent->>Orchestrator: Результат вычисления
    end
    Orchestrator->>User: Возврат результата
```



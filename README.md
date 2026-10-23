# Gateway

HTTP gateway на Go: загрузка и подготовка YAML-конфигурации (control plane), маршрутизация, reverse proxy и цепочки внешних интерцепторов. Envoy, xDS, Lua, domain-слой, БД и брокер не используются.

## Запуск

Инструменты и задачи настроены через `mise`, как в `core` и шаблоне модулей:

```sh
cd gateway
mise install
mise run generate
mise exec -- go run ./cmd --config config check-config
mise exec -- go run ./cmd --config config service
```

Рабочий минимальный пример находится в `config/`. `/gateway/status` возвращает локальный ответ, остальные пути проксируются в `localhost:8080`. Upstream можно изменить переменной `GATEWAY_UPSTREAM`. Примеры цепочки session → authz → error-pages находятся в `docs/configuration/examples/`; для их валидации требуется `AUTHZ_API_KEY`, для выполнения — соответствующие внешние сервисы.

Переменные процесса:

| Переменная | Назначение |
|------------|------------|
| `GATEWAY_CONFIG` | YAML-файл или каталог; default `config` |
| `GATEWAY_HTTP_ADDRESS` | адрес proxy listener; default определяется `port` |
| `GATEWAY_HEALTH_ADDRESS` | адрес health listener; default определяется `healthzPort` |

CLI использует `urfave/cli/v3`, как модули core: `service` — команда по умолчанию, `check-config` проверяет конфигурацию. Флаг `--check-config` также поддерживается.

Endpoint на health listener: `/healthz`, `/readyz`. SIGINT/SIGTERM выключают readiness и дают текущим запросам до 30 секунд на завершение. При buffered request body чтение ограничено 60 секундами; streaming passthrough этого ограничения не имеет.

```sh
mise run build
# Linux-бинарник: dist/gateway
# Для запуска на macOS используйте go run или go build ./cmd.
mise run image
docker compose up --build
```

## Контракт

- [Конфигурация и порядок merge](docs/configuration/README.md).
- [Маршруты и цепочки](docs/configuration/servicedefinition.md).
- [API внешних интерцепторов](docs/api/README.md).

Маршруты более высокого веса проверяются первыми; внутри документа порядок сохраняется. Rewrite/internal redirect предшествуют request-интерцепторам. Каждый следующий интерцептор видит уже применённые mutations предыдущего. `set` заменяет все прежние значения заголовка, `append` выполняется явно. После немедленного ответа цепочки прекращаются и upstream не вызывается, если запрос ещё не отправлен.

Транспорты интерцепторов — gRPC и REST. Контракты в `api` являются источником генерации Go-клиентов и gRPC server API в `api/client`. Поддерживаются request/response headers, полные buffered bodies и trailers. Buffered bodies ограничены размером (default 1 MiB, максимум настройки 32 MiB); REST использует base64. Streaming-body modification пока не поддерживается. Без body processing запросы и ответы передаются потоком, включая SSE, HTTP/2 и gRPC с trailers. Middleware специфики session, CSRF, authorization, maintenance, CSP и error-pages находится во внешних сервисах.

Локально реализованы header matching, allowedMethods, rewrite, internal redirect, directResponse, host rewriting, динамические addHeaders, Basic Auth (htpasswd SHA/bcrypt), env/secret substitutions и таймауты. `timeout: 0s` и `idleTimeout: 0s` выключают соответствующий таймер. Idle timeout измеряет отсутствие данных от upstream после отправки запроса, а не срок свободного соединения в пуле.

`gatewayHealthHost` сохранён как необязательный дополнительный health listener (default 8082, `0` выключает); минимальный пример его выключает. Управляющий API на `gatewayPort` не определён: ненулевое значение отклоняется при валидации. Hot reload и `dynamicTarget` не реализованы.

## Проверки

```sh
mise run all
# Отдельно:
mise run test
mise run integration-tests
mise run lint
mise run coverage # dist/coverage.out и dist/coverage.html
```

Интеграционные тесты открывают реальные локальные TCP-порты для gateway, HTTP/gRPC backend и REST/gRPC интерцепторов. Docker не требуется. Проверяются фактические запросы upstream и ответы клиенту: маршруты/merge, query и бинарное тело, session → authz, SetOrReplaceHeaders, ранний отказ, модификации headers/body/trailers, внешние error-pages, лимиты, ошибки/таймауты, отмена, SSE, gRPC unary/streaming и статусы/trailers. Дополнительно запускается собранный executable с YAML; проверяются readiness, остановка по сигналу и завершение текущего запроса при shutdown. Интеграционная задача выполняется с `-race`.

Полная [матрица проверяемого поведения](docs/testing.md) связывает возможности конфигурации и API с тестами. Тесты используют `testify/require`, `testify/assert` и `testify/suite`. В обработчиках локальных серверов используются неблокирующие assertions; `require` выполняется в goroutine теста. Задача coverage измеряет app/config/infra, исключая сгенерированные клиенты; CLI проверяется отдельной suite и запуском executable.

Реализация разделена по этапам: `app` — выбор маршрута, request/response processing, цепочка и валидация mutations; `config` — чтение документов, merge, подстановки и компиляция правил; `infra` — транспорты, таймер активности и lifecycle. Файлы API-клиентов генерируются из контрактов и вручную не разбиваются.

## Сборка и релиз

Схемы хранятся в `api/interceptorgrpc` и `api/interceptorrest`. `mise run generate`
создаёт Go-код в `api/client`; он исключён из Git. Задачи сборки и проверок сами
запускают генерацию, поэтому работают и после чистого clone.

`mise run build` собирает `dist/gateway` для Linux/amd64 с `CGO_ENABLED=0`.
Архитектуру можно изменить: `GOARCH=arm64 mise run image`.
`mise run image` собирает `gateway:local`; имя задаётся переменной `IMAGE`.
Для отправки в registry: `IMAGE=ghcr.io/distributed-programming-2026/gateway:v0.0.1 mise run image-push`
(предварительно выполните `docker login ghcr.io`).

GitHub Actions выполняет `mise run all` и собирает Docker-образ на pull request и
push в `main`. Тег `vMAJOR.MINOR.PATCH` запускает те же проверки, затем публикацию
`ghcr.io/distributed-programming-2026/gateway:<tag>` и GitHub Release.
Используется `GITHUB_TOKEN` с правами `packages: write` и `contents: write`;
отдельные секреты для registry не нужны. Образ собирается для Linux/amd64.

Разработчик публикует релиз из чистой ветки `main`, совпадающей с `origin/main`:

```sh
git pull --ff-only
mise install
mise run release -- v0.0.1
```

Задача проверяет формат версии и отсутствие тега, выполняет `mise run all`,
проверяет отсутствие изменений в отслеживаемых файлах, создаёт аннотированный
тег и отправляет его в `origin`. Публикация образа и GitHub Release выполняется
асинхронно: результат нужно проверить в Actions. При ошибке push локальный тег
сохраняется для повторной отправки; существующие теги задача не перезаписывает.

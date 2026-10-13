# Service Definition

Маршруты сервиса и объявление внешнего интерцептора. `kind` — `gateway/service/v1`. Все regexp используют Go `regexp`; [общие правила загрузки и обработки](README.md) обязательны для этого документа.

## Поля документа

| Поле | Тип | Обязательное | Описание |
|------|-----|--------------|----------|
| `kind` | string | да | `gateway/service/v1` |
| `weight` | int | нет | приоритет, default `0`; маршруты большего веса проверяются раньше |
| `name` | string | да | имя сервиса и идентификатор объявленного интерцептора |
| `interceptor` | object | нет | внешний gRPC или REST интерцептор |
| `routes` | array | для сервиса с маршрутами | упорядоченный список маршрутов; можно опустить для объявления только интерцептора |

Документ должен содержать непустой `routes` или `interceptor`. Маршруты проверяются в порядке объявления. При merge маршруты конкатенируются блоками от большего веса к меньшему, включая документы одинакового `name`. При равных весах порядок документов может быть любым; внутри документа порядок сохраняется.

## Пример

```yaml
kind: gateway/service/v1
weight: 100
name: backend
routes:
  - pattern: '^/backend/api/users/[^/]+$'
    rewrite:
      old: '^/backend/(.*)$'
      new: '/$1'
    target: backend:8082
    allowedMethods: [GET, POST]
    timeout: 60s
    interceptors:
      - id: session
      - id: authz
        pathParametersPattern: '^/api/users/{user_id}$'
        parameters:
          action: users.read

  - pattern: '^/backend/stream/.*$'
    target: backend:8082
    timeout: 0s
    idleTimeout: 0s
```

Определения `session` и `authz` находятся в отдельных service-документах; см. [готовые примеры](examples/).

## Объявление внешнего интерцептора

```yaml
kind: gateway/service/v1
name: authz
interceptor:
  address: authz:8081
  transport: grpc
  timeout: 5s
```

| Поле | Тип | По умолчанию | Описание |
|------|-----|--------------|----------|
| `address` | string | — | обязательно; `host:port` для gRPC, базовый HTTP(S) URL для REST |
| `transport` | string | `grpc` | `grpc` или `rest` |
| `timeout` | duration | `1m` | срок одного вызова Intercept; `0s` отключает таймаут |

REST-пример: `address: http://authz:8082`. Gateway вызывает `POST /gateway/api/v1/intercept` относительно этого базового URL. Путь endpoint фиксирован предоставленным OpenAPI-контрактом. Для gRPC используется RPC `interceptorgrpc.Interceptor/Intercept`. Таймаут применяется к одному вызову фазы.

## Поля маршрута

| Поле | Тип | Описание |
|------|-----|----------|
| `pattern` / `patterns` | string / []string | обязательно ровно одно из полей; Go regexp для пути |
| `target` | string | upstream `host:port` или HTTP(S) URL; обязательно для проксирования |
| `rewrite` | object | изменение пути в рамках выбранного маршрута |
| `internalRedirect` | object | изменение пути с повторным выбором маршрута |
| `timeout` / `idleTimeout` | duration | переопределение глобальных таймаутов; `0s` отключает соответствующий таймаут |
| `headerMatch` | array | дополнительные условия выбора маршрута |
| `allowedMethods` | []string | ограничение методов уже выбранного маршрута |
| `interceptors` | array | последовательная цепочка внешних интерцепторов с настраиваемыми request/response-фазами |
| `directResponse` | object | локальный ответ после выполнения интерцепторов |
| `disableHostRewrite` | bool | сохранить исходный Host; default `false` |
| `useHTTP2` | bool | HTTP/2 к upstream, включая gRPC; default `false` |
| `addHeaders` | map[string]Header | заголовки ответа |
| `basicAuth` | object | локальная проверка Basic Auth по users / htpasswd |

Для конечного маршрута задаётся ровно одно из `target` и `directResponse`. Маршрут с `internalRedirect` может не иметь ни одного: если правило перехода не совпало и конечного действия нет, gateway возвращает `404`. `dynamicTarget` отсутствует в контракте. Error pages подключается через внешние интерцепторы, встроенного флага `errorPages` нет.

### pattern / patterns и headerMatch

Сопоставляется текущий URL path без query. `patterns` — непустой список альтернатив внутри одной позиции маршрута. После совпадения пути все условия `headerMatch` должны выполниться:

```yaml
patterns:
  - '^/api/v1/.*$'
  - '^/api/v2/.*$'
headerMatch:
  - header: Host
    valueRegex: '^api\.example\.com(:[0-9]+)?$'
  - header: X-Region
    valueRegex: '^ru$'
```

Имена заголовков сравниваются без учёта регистра; `Host` берётся из host запроса. Отсутствующий заголовок не совпадает, для повторяющихся значений достаточно совпадения одного. Если путь или заголовки не совпали, проверяется следующий маршрут.

### allowedMethods

```yaml
allowedMethods: [GET, POST]
```

Отсутствующий или пустой список разрешает любые методы. Иначе метод проверяется после выбора маршрута по пути и заголовкам. При запрете gateway возвращает `403 Forbidden`, не вызывает интерцепторы и upstream и не ищет следующий маршрут. `GET` также разрешает `HEAD`; `OPTIONS` разрешается только при явном указании.

Это выбранное поведение по образцу nginx `limit_except … { deny all; }`: ограничение применяется внутри выбранного location, а запрещённые методы получают `403`. См. [limit_except](https://nginx.org/en/docs/http/ngx_http_core_module.html#limit_except) и [пример NGINX с ответом 403](https://docs.nginx.com/nginx-gateway-fabric/traffic-management/snippets/).

### rewrite

```yaml
rewrite:
  old: '^/old/([^/]+)$'
  new: '/new/$1'
```

Go regexp для `old`; стандартный Go replacement для `new`: `$1`, `$2` и далее для обычных групп, `${name}` для именованных. Формат `{name}` здесь не раскрывается. Применяется к текущему пути до интерцепторов. Если `old` не совпал, путь сохраняется. Rewrite сам по себе не запускает повторный выбор маршрута. Query сохраняется; replacement описывает только путь.

### internalRedirect

```yaml
internalRedirect:
  old: '^/legacy/([^/]+)$'
  new: '/api/users/$1'
  maxRedirectCount: 10
```

Использует Go regexp и стандартный replacement `$1` / `${name}`, как rewrite. Выполняется после `rewrite`, до внешних интерцепторов. При совпадении изменяет путь и повторяет выбор маршрута, проверку метода и встроенные операции. Интерцепторы вызываются только на конечном маршруте. Клиенту не возвращается HTTP redirect; метод, query и тело сохраняются.

Default `maxRedirectCount: 10`. Значение должно быть положительным. Счётчик общий для запроса; при каждом встреченном правиле его лимит уменьшает общий допустимый предел (до минимума из встреченных лимитов). Попытка превысить предел завершает запрос статусом `500` без вызова интерцепторов и upstream. Это ограничивает в том числе циклы между разными маршрутами. Несовпавшее правило перехода не расходует счётчик.

### timeout / idleTimeout

Отсутствующее поле наследует значение `proxyTimeouts`; явное `0s` отключает таймер. `timeout` ограничивает всю операцию проксирования. `idleTimeout` ограничивает ожидание данных от upstream после отправки запроса, включая ожидание первого ответа и интервалы между данными. Полученные данные перезапускают idle-таймер. Подробности в [Gateway Definition](gatewaydefinition.md#proxytimeouts).

Если таймаут наступил до отправки заголовков клиенту — `504`; после начала ответа передача прерывается, заменить уже отправленный статус нельзя. Недоступность upstream до начала ответа — `502`. Gateway не выполняет прикладные повторы запросов; стандартный HTTP transport может безопасно повторить replayable запрос при сбое переиспользуемого соединения.

### target, disableHostRewrite, useHTTP2

`target: backend:8082` означает HTTP upstream. Можно явно указать `http://backend:8082` или `https://backend:8443`; target содержит только scheme и authority, путь задаётся запросом и rewrite.

По умолчанию Host переписывается на authority upstream. `disableHostRewrite: true` сохраняет исходный Host. `useHTTP2: true` требует HTTP/2 к upstream: h2c для HTTP, TLS HTTP/2 для HTTPS. Для gRPC проксируются metadata, тело потока и trailers, включая `grpc-status` и `grpc-message`.

### directResponse

```yaml
directResponse:
  status: 403
  inlineString: Forbidden
```

Статус от `200` до `599`. Ответ формируется после request-фаз конечного маршрута и проходит включённые response-фазы; backend не вызывается. `immediateResponse` интерцептора завершает обработку раньше и заменяет это действие.

### interceptors

Каждая запись ссылается на внешний интерцептор через обязательный `id`. Поле `type` отсутствует: все интерцепторы внешние. Параметры и `processingMode` опциональны.

```yaml
interceptors:
  - id: session
  - id: authz
    pathParametersPattern: '^/api/users/{user_id}$'
    parameters:
      action: users.read
      apiKey: secret:authz:api_key
```

| Поле | Тип | Описание |
|------|-----|----------|
| `id` | string | обязательно; имя service-конфигурации с блоком `interceptor` |
| `pathParametersPattern` | string | шаблон `{name}`, преобразуемый в Go regexp с именованными группами; можно также задать группы явно |
| `parameters` | map[string]string | статические параметры; поддерживаются env и secret ссылки |
| `processingMode` | object | включённые request/response headers, buffered body и trailers; defaults и лимиты в [API](../api/README.md#настройка-вызовов) |

Ссылка на отсутствующий интерцептор — ошибка при запуске. Специальная обработка `{name}` применяется в полях `pathParametersPattern` интерцепторов и заголовков; обычные regexp-поля остаются без преобразований. `pathParametersPattern` преобразуется из шаблона `{name}` в Go regexp при запуске: `{user_id}` → `(?P<user_id>[^/]+)`. `pathParameters` в запросе внешнему интерцептору содержит именованные группы первого совпадения с текущим преобразованным путём; если поле отсутствует или regexp не совпал, словарь пуст. Обычные неименованные группы в словарь не включаются. Имена и экранирование описаны в [общем контракте](README.md#regexp-и-параметры-пути). Query передаётся в `context.request.rawQuery` без потери повторяющихся значений, порядка и escaping.

Контракты: [gRPC](../api/interceptorgrpc/interceptorgrpc.proto), [REST/OpenAPI](../api/interceptorrest/interceptorrest.oapi.yaml) и [семантика API](../api/README.md). Оба транспорта поддерживают одни и те же фазы и решения.

В каждой включённой фазе интерцепторы вызываются в порядке объявления. После каждого `continueResponse` gateway немедленно применяет mutations к текущему состоянию. Следующий интерцептор получает уже изменённые заголовки, method/path/query и тело, если соответствующая фаза передаёт его. Например, session устанавливает `X-Auth-Account` / `X-Auth-User`, а следующий authz получает эти данные без дополнительной загрузки сессии.

Операция `headerMutation.set` следует правилу SetOrReplaceHeaders: заменяет все прежние значения имени (без учёта регистра). Более поздний интерцептор заменяет значение предыдущего. Добавление значений требует явного `append`; удаление — `remove`. То же правило действует для response headers и trailers. Следующий участник никогда не получает неизменённую копию исходного запроса вместо текущего состояния.

`continueResponse` без mutations разрешает продолжить обработку. `immediateResponse` возвращает готовый ответ и завершает все дальнейшие фазы; request upstream не отправляется, если это произошло до отправки. Ошибка вызова или невалидное решение — `502`, timeout — `504`, с остановкой текущей цепочки и без повторов. Правила обработки локальных ошибок response-интерцепторами описаны в API.

Отложенные `responseHeaderMutation` request-фаз применяются к ответу до response-фаз: response-интерцептор видит их и может заменить. Response-фазы вызываются до отправки соответствующих данных клиенту; `addHeaders` накладывается после них.

Пример внешнего error-pages без request-вызова:

```yaml
interceptors:
  - id: error-pages
    processingMode:
      requestHeaders: false
      responseHeaders: true
```

### addHeaders

```yaml
addHeaders:
  X-Static-Header:
    value: static-value
  X-User-ID:
    pathParametersPattern: '^/api/users/{user_id}$'
    value: 'user_id={user_id}'
  X-Always-Header:
    value: always-value
    always: true
```

Без `pathParametersPattern` значение `value` статическое. Если поле задано, gateway преобразует `{name}` в именованные Go-группы, извлекает значения из конечного пути после rewrite и internal redirect и подставляет их в `value` через `{name}`. Это локальная операция без вызова интерцептора. При несовпадении пути заголовок пропускается; неизвестное имя параметра в `value` — ошибка конфигурации. Поддерживаются и явно именованные Go-группы. Env/secret-подстановки выполняются при загрузке.

Например, для `/api/users/42` пример выше устанавливает `X-User-ID: user_id=42`. Правила placeholder и экранирования общие с интерцепторами; подстановки rewrite `$1` к `addHeaders.value` не относятся.

По умолчанию заголовок устанавливается для статусов `200–299`, с `always: true` — для всех статусов. Заголовок заменяет существующие значения. Для HEAD применяются те же правила, но тело ответа клиенту не передаётся.

Приоритет заголовков от меньшего к большему: backend/direct response → отложенные responseHeaderMutation request-фаз → mutations response-фаз → `addHeaders`. Для immediateResponse его собственные заголовки накладываются после отложенных mutations; дальнейшие response-фазы не вызываются. Имена сравниваются без учёта регистра. Правила работают для ответа конечного маршрута, включая ошибки и immediateResponse.

### basicAuth

Локальная Basic Auth сохраняется из исходного примера; обращение во внешний сервис не требуется.

```yaml
basicAuth:
  users:
    - 'user1:{SHA}EIRU7AdrXt4H/...'
    - secret:basic_auth:cred
```

`users` — непустой список htpasswd-записей `username:password-hash` или secret-ссылок на такие записи. Проверка выполняется до внешних интерцепторов. При отсутствующих или неверных credentials возвращается `401` с `WWW-Authenticate: Basic realm="gateway"`; request-интерцепторы и backend не вызываются; локальный ответ может пройти response-интерцепторы. Поддерживаются htpasswd SHA (`{SHA}` + base64 SHA-1 digest) и bcrypt (`$2a$`, `$2b$`, `$2y$`); неподдерживаемый hash отклоняется при запуске. Пример хеша выше иллюстрирует формат; для запуска требуется полная запись.

# API внешних интерцепторов

Контракт первой реализации Go gateway. Go выполняет обе роли: control plane загружает, объединяет и валидирует YAML, готовит маршруты и клиентов; HTTP-сервер исполняет эту конфигурацию и проксирует запросы. Envoy, xDS и Lua сейчас не используются: для ожидаемой нагрузки отдельный proxy/control plane стек не требуется.

## Связь с исходной архитектурой

Исходные `okResponse.headersToSet`, `responseHeadersToSet` и `forwardResponse` соответствуют модели Envoy ext_authz: разрешить запрос с изменениями или вернуть локальный ответ. Сам ext_authz уже поддерживает изменение заголовков и query при успешной авторизации. См. [контракт ext_authz](https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/auth/v3/external_auth.proto).

Из ext_proc заимствуется модель отдельных событий для request/response headers, body и trailers, решения продолжить обработку с mutations и немедленного ответа. См. [контракт ext_proc](https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/ext_proc/v3/external_processor.proto) и [processing mode](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_proc/v3/processing_mode.proto).

Это собственный API, а не реализация wire-протокола Envoy: один unary RPC или REST-вызов на событие, одинаковая семантика обоих транспортов. Первая версия поддерживает body `none` / `buffered`; chunk streaming, full duplex, runtime mode override и повторный выбор маршрута по решению интерцептора не входят в этот черновик.

## Транспорты и совместимость

- gRPC: `interceptorgrpc.Interceptor/Intercept`, [proto](interceptorgrpc/interceptorgrpc.proto).
- REST: `POST /gateway/api/v1/intercept`, [OpenAPI](interceptorrest/interceptorrest.oapi.yaml). Запрос содержит `eventType`, `context` и `event`. Ответ HTTP `200` содержит `payload` с `responseType: continueResponse` или `immediateResponse`.

Контракт заменяет исходный draft и несовместим с ним по сообщениям. Уже работающего API в этом проекте нет; имя RPC и endpoint сохранены. Старые protobuf field numbers зарезервированы. Перед публикацией API необходимо закрепить release version; REST не использует protobuf JSON напрямую.

## События

| Событие | Когда вызывается | Данные |
|---------|------------------|--------|
| `requestHeaders` | после локальных преобразований маршрута, перед upstream | текущие заголовки запроса |
| `requestBody` | перед отправкой запроса, если включена буферизация | всё тело запроса |
| `requestTrailers` | после чтения тела запроса | текущие request trailers |
| `responseHeaders` | после получения ответа, до отправки клиенту | текущий HTTP status и заголовки |
| `responseBody` | до отправки ответа, если включена буферизация | всё тело ответа |
| `responseTrailers` | после чтения тела ответа | текущие response trailers |

Каждое сообщение содержит ровно одно событие. `context` содержит `requestId`, текущие method/scheme/authority/path/rawQuery/headers, статические `parameters` и извлечённые `pathParameters` конкретного интерцептора. `rawQuery` сохраняет повторы, порядок и escaping без начального `?`; старый словарь `queryParameters` заменён им, чтобы не терять информацию. Параметры пути вычисляются заново перед каждым вызовом из текущего пути.

Request/response headers и trailers представлены массивом пар: сохраняются повторяющиеся значения. В заголовках нет pseudo-headers и Host: authority, method и path представлены отдельными полями контекста. `endOfStream` означает отсутствие последующих тела/trailers, а не отсутствие будущих фаз в конфигурации; для body пустой массив байтов — полноценное пустое тело.

Тела — произвольные bytes (protobuf) / base64 (REST). Content-Encoding автоматически не раскрывается: интерцептор получает фактические байты тела. Он отвечает за соответствующие Content-Type/Content-Encoding при замене.

## Настройка вызовов

В каждой записи `route.interceptors` можно задать `processingMode`:

```yaml
interceptors:
  - id: authz
    processingMode:
      requestHeaders: true
  - id: modifier
    processingMode:
      requestHeaders: true
      requestBody: buffered
      responseHeaders: true
      responseBody: buffered
      requestTrailers: true
      responseTrailers: true
      maxRequestBodyBytes: 1048576
      maxResponseBodyBytes: 1048576
  - id: error-pages
    processingMode:
      requestHeaders: false
      responseHeaders: true
```

Defaults: `requestHeaders: true`, `responseHeaders: false`, оба body mode `none`, оба trailers `false`, оба лимита `1048576` байт. Лимит должен быть от 1 до 33554432 байт (32 MiB). Таймаут объявления `interceptor.timeout` действует на каждый вызов отдельно; отключается через `0s`. Таймаут маршрута ограничивает проксирование upstream; request-вызовы до начала проксирования имеют собственные таймауты. Response processing входит в общее время проксирования. При буферизации request body gateway устанавливает read deadline 60 секунд и снимает его после чтения.

В каждой фазе включённые интерцепторы вызываются в порядке `route.interceptors`, включая response-фазы. Сначала выполняется вся headers-фаза, потом body, потом trailers; следующий участник фазы видит mutations предыдущего. Gateway изменяет общий текущий запрос/ответ сразу после каждого решения, а не откладывает изменения до конца цепочки. Например, session на requestHeaders устанавливает X-Auth-User, следующий authz в той же фазе получает его. Для совпадающих имён set работает как SetOrReplaceHeaders: последнее значение заменяет все предыдущие; append выполняется только по явному указанию. Это явно выбранный порядок собственного API, не обещание совпадения порядка со стеком фильтров Envoy.

Если хотя бы один участник требует buffered body, gateway буферизует соответствующее направление. Лимит направления — минимальный из лимитов всех участников, включённых в этом направлении; он применяется к исходному и изменённому телу. Request headers обрабатываются до чтения buffered request body, чтобы авторизация могла отклонить запрос без его чтения. При request buffering тело не отправляется upstream до успешного завершения request-фаз. При response buffering заголовки и тело не отправляются клиенту до успешного завершения response-фаз.

Request trailers processing в первой версии требует request body `buffered`; response trailers processing требует response body `buffered`. Иначе конфигурация невалидна: unary API не должен неявно включать buffering streaming-запроса. При наличии соответствующих trailers вызываются включённые обработчики; отсутствие trailers не создаёт события.

При mode `none` тела не читаются для интерцепторов и передаются потоком. Headers processing совместим с gRPC/HTTP/2, SSE и большими загрузками. Body/trailer processing нельзя включать на маршрутах с `useHTTP2: true` в первой версии: gRPC потоки и фреймы gateway проксирует без модификации. WebSocket после upgrade также не проходит body processing.

## Решение интерцептора

Ровно один результат: `continueResponse` или `immediateResponse`. Отсутствие результата или mutation, недопустимая для текущей фазы, — ошибка интерцептора. `continueResponse` без mutations разрешает продолжить обработку; отдельного allow-флага не требуется.

### ContinueResponse

| Поле | Разрешённые фазы | Действие |
|------|-----------------|----------|
| `headerMutation` | requestHeaders / responseHeaders | удалить, заменить или добавить значения заголовков |
| `requestMutation` | requestHeaders | заменить method, authority, path или rawQuery |
| `bodyMutation` | requestBody / responseBody; также headers соответствующего направления | заменить тело целиком, включая пустое |
| `trailerMutation` | requestTrailers / responseTrailers | изменить trailers |
| `statusCode` | responseHeaders | заменить HTTP status, `200–599` |
| `responseHeaderMutation` | любая request-фаза | отложенные изменения заголовков ответа, как в исходном ext_authz API |

`HeaderMutation` применяет операции в порядке `remove` → `set` → `append`. `set` заменяет все значения имени; `append` сохраняет существующие и добавляет новые. `values` — непустой список; пустое значение допускается как `values: [""]`. Повторяющиеся имена в одном списке `set`/`append` запрещены. Имена сравниваются без учёта регистра; Set-Cookie сохраняется отдельными значениями.

Method/authority/path/rawQuery изменяются через отдельные optional-поля: отсутствие сохраняет значение, `rawQuery: ""` очищает query. Path должен быть абсолютным путём без query/fragment, authority — корректным host[:port], method — HTTP token. Scheme, upstream target и выбранный маршрут не изменяются. Rewrite/internalRedirect и already completed auth checks повторно не выполняются; следующая авторизация получает изменённый запрос. Изменение authority при отправке upstream подчиняется `disableHostRewrite`.

Body mutation в body-фазе требует mode `buffered`. Body mutation в headers-фазе может заменить тело без чтения исходного (например, страницу ошибки): исходный stream закрывается, новое тело становится полным body соответствующего направления. Настроенные body/trailer-фазы исходного тела для этого направления пропускаются, но оставшиеся headers-интерцепторы вызываются и видят новое состояние. Replacement не требует mode `buffered`, но подчиняется лимиту размера. В первой версии headers body replacement запрещён для gRPC и upgrade.

При замене тела gateway пересчитывает Content-Length, очищает старые trailers и их объявления, удаляет старые ETag/Content-MD5/Digest; не доверяет присланным framing-заголовкам. Hop-by-hop, Host, pseudo-headers, Content-Length, Transfer-Encoding и Trailer нельзя изменять через HeaderMutation. Trailer mutation не может устанавливать framing/authorization поля. Некорректные имена/значения и запрещённые операции вызывают ошибку, а не молча игнорируются.

Влияние mutation на `endOfStream` пересчитывается gateway перед следующим вызовом. HTTP-правила отсутствия тела для HEAD, 204 и 304 применяются при отправке клиенту независимо от решения интерцептора.

### ImmediateResponse

Готовый HTTP-ответ: status `200–599`, headers и bytes body. В request-фазе обеспечивает отказ авторизации, redirect или другой локальный ответ; upstream не вызывается, если запрос ещё не отправлен. В response-фазе заменяет текущий ответ, например страницу ошибки.

Результат завершает все дальнейшие вызовы интерцепторов этого запроса, включая response-фазы. Ответ проходит только локальные addHeaders и правила HTTP. Отложенные responseHeaderMutation выполненных request-фаз применяются перед собственными заголовками immediate response; заголовки immediate имеют приоритет. Автоматического повторного вызова error-pages для такого ответа нет.

В первой версии все разрешённые body/trailer-фазы буферизованы, поэтому immediate response возможен до отправки ответа клиенту. Замена статуса/заголовков после начала streaming-ответа не предусмотрена.

## Ошибки и error pages

Отказ авторизации — корректный `immediateResponse`, а не ошибка RPC или REST. Ошибка транспорта/некорректное решение — `502`, timeout вызова — `504`. Продолжения request-цепочки и автоматических повторов нет. Превышение request buffer limit — `413`, response buffer limit — `502`; усечённое тело интерцептору не передаётся.

Response headers processing применяется к upstream, directResponse и локальным ошибкам gateway, если конечный маршрут уже известен. Контекст локальной ошибки содержит `gatewayError.code`; детали ошибок и секреты наружу не передаются. Отсутствие маршрута не вызывает интерцепторы. ImmediateResponse прекращает обе цепочки и исключает этот вызов.

Если request-фаза завершилась ошибкой, gateway может обработать сформированный локальный ответ через настроенную response-цепочку. Интерцептор, уже давший ошибку, исключается. Если ошибка возникает внутри response-цепочки, она прекращается и возвращается локальный 502/504 без повторного запуска цепочки. Ошибка буферизации ответа также возвращает локальный 502 без повторного запуска. Это исключает рекурсию и повторные вызовы неисправного сервиса.

Error-pages на `responseHeaders` получает status и может вернуть `continueResponse` с новым телом, Content-Type и сохранённым статусом; либо `immediateResponse`. Читать исходное тело для генерации страницы по статусу не требуется. Отдельного встроенного `errorPages.address` или route-флага `errorPages` нет.

## Maintenance и CSP

Оба механизма реализуются внешними интерцепторами. Встроенных maintenance/CSP полей и обработчиков в gateway нет.

Maintenance включается в requestHeaders после session, если ему нужны данные аккаунта. Он получает уже установленные сессионные заголовки и возвращает `immediateResponse` (например, `503`) либо `continueResponse` с нужными заголовками. Исключения для маршрутов задаются составом цепочки и статическими `parameters` интерцептора.

CSP может использовать requestHeaders для подготовки nonce и передачи его backend, а responseHeaders — для установки Content-Security-Policy и обработки служебных заголовков backend. Согласование nonce и политика остаются ответственностью внешнего интерцептора. Данные между его фазами можно передать через изменённые заголовки текущего запроса, доступные в `context.request`; gateway не содержит CSP-специфического состояния.

После `immediateResponse` response-интерцепторы не вызываются по общему правилу завершения цепочки. Если локальной странице maintenance/отказа нужна CSP, её должен задать формирующий ответ интерцептор либо выполненный ранее request-интерцептор через отложенный `responseHeaderMutation`.

## Примеры решений REST

Разрешить запрос, удалить credentials и добавить principal:

```json
{"payload":{"responseType":"continueResponse","headerMutation":{"remove":["Authorization"],"set":[{"key":"X-Auth-User","values":["42"]}]}}}
```

Отклонить авторизацию:

```json
{"payload":{"responseType":"immediateResponse","statusCode":403,"headers":[{"key":"Content-Type","value":"text/plain"}],"body":"Rm9yYmlkZGVu"}}
```

Заменить HTML страницы ошибки в responseHeaders с сохранением статуса:

```json
{"payload":{"responseType":"continueResponse","headerMutation":{"set":[{"key":"Content-Type","values":["text/html; charset=utf-8"]}]},"bodyMutation":{"data":"PGgxPkVycm9yPC9oMT4="}}}
```

Интеграционными тестами проверяются: allow/deny без вызова backend при deny, последовательные mutations, request и response body replacement (включая пустой/binary), response-only error-pages, header set/append/remove, сохранение query/Set-Cookie, отключённые фазы, превышение лимитов, timeout/cancel, отсутствие рекурсии и streaming passthrough без буферизации.

Пример REST-события, которое получает authz после session:

```json
{"eventType":"requestHeaders","context":{"requestId":"req-42","request":{"method":"GET","scheme":"http","authority":"example.com","path":"/api/users/42","rawQuery":"","headers":[{"key":"X-Auth-User","value":"42"}]},"pathParameters":{"user_id":"42"},"parameters":{"action":"users.read"}},"event":{"headers":[{"key":"X-Auth-User","value":"42"}],"endOfStream":true}}
```

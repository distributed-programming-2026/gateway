# Проверки gateway

Таблица связывает реализованный контракт с тестами. Интеграционные сценарии запускают реальные HTTP/gRPC upstream, REST/gRPC интерцепторы и gateway на локальных TCP-портах, проверяя данные на обоих концах проксирования. TLS upstream проверяется отдельным транспортным тестом с доверенным тестовым сертификатом. Assertions — `testify/assert` и `testify/require`; конфигурация и CLI используют `testify/suite` с `SetupTest`.

## Запуск

```sh
mise run all
mise run coverage
```

`all` выполняет генерацию API, tidy, Linux build, unit/CLI tests, интеграционные тесты с `-race` и обновлённый строгий линтер. `coverage` запускает весь набор с `-race` и измеряет handwritten app/config/infra. Отчёты: `dist/coverage.out`, `dist/coverage.html`; сгенерированные API-клиенты исключены из измерения. Запуск executable, включая сигналы и адреса listener, проверяется отдельно; его subprocess не входит в процент покрытия процесса тестов.

## Конфигурация

| Возможность | Проверка |
|---|---|
| Высокий вес первым, порядок routes внутри документа, несколько документов одного name | [ConfigSuite/TestMergePreservesZeroEmptyListAndRouteWeights](../internal/gateway/config/merge_test.go), [TestRoutingRewriteAndProxy](../internal/integration-tests/routing_test.go) |
| Рекурсивный merge объектов, замена списков, сохранение 0s/пустого списка | [ConfigSuite/TestMergePreservesZeroEmptyListAndRouteWeights](../internal/gateway/config/merge_test.go), [TestDefaultsAndInterceptorMerge](../internal/gateway/config/defaults_test.go) |
| Загрузка каталога, `.yaml`/`.yml`, многодокументный YAML и default после merge | [TestDefaultsAndInterceptorMerge](../internal/gateway/config/defaults_test.go), [TestMissingAndEmptyConfiguration](../internal/gateway/config/validation_test.go) |
| Merge объявлений interceptor одного name, наследование address/transport и отключение timeout | [TestDefaultsAndInterceptorMerge](../internal/gateway/config/defaults_test.go) |
| Secret min/max, сохранение незаменённых ключей, env/default и подстановки в списках | [ConfigSuite/TestSecretsAndEnvironment](../internal/gateway/config/secrets_test.go), [TestSecretsMergeRetainsKeysAndExpandsLists](../internal/gateway/config/defaults_test.go) |
| `{name}`, явные именованные группы, quantifiers, literal braces, character classes, `\Q…\E` | [TestParameterRegexpSyntaxAndHeaderTemplates](../internal/gateway/config/parameters_test.go) |
| Динамический header value, literal `{{`/`}}`, несовпадающий путь, статическое значение | [TestParameterRegexpSyntaxAndHeaderTemplates](../internal/gateway/config/parameters_test.go), [TestAddHeadersSuccessAlwaysLiteralAndMismatch](../internal/integration-tests/routing_contract_test.go) |
| Неизвестные/удалённые поля, null, kind, weight, порты, duration, targets, regexp, методы, заголовки, ссылки на interceptor, body mode/limits, htpasswd, env/secret ошибки | Таблицы [TestRejectedDefinitions](../internal/gateway/config/validation_test.go), [TestMissingAndEmptyConfiguration](../internal/gateway/config/validation_test.go) |
| `gatewayHealthHost: 0` выключает alias; nonzero gatewayPort отклоняется | [TestDefaultsAndInterceptorMerge](../internal/gateway/config/defaults_test.go), [TestRejectedDefinitions](../internal/gateway/config/validation_test.go), [TestExecutableStartsWithYAMLAndStopsOnSignal](../internal/integration-tests/executable_test.go) |

При равных весах порядок между документами не является частью контракта и тестами не фиксируется.

## Маршрутизация и локальные функции

| Возможность | Проверка |
|---|---|
| Первый совпавший route, `pattern`/`patterns`, fallback и отсутствие совпадения | [TestRoutingRewriteAndProxy](../internal/integration-tests/routing_test.go), [TestPatternsHostAndHeaderConjunction](../internal/integration-tests/routing_contract_test.go) |
| Все headerMatch одновременно, Host, repeated headers | [TestPatternsHostAndHeaderConjunction](../internal/integration-tests/routing_contract_test.go), [TestMethodsHostMatchingAndRedirectLoop](../internal/integration-tests/route_selection_test.go) |
| Rewrite `$1`/`${name}`, rewrite до internalRedirect, повторный выбор route, builtins до interceptor | [TestRoutingRewriteAndProxy](../internal/integration-tests/routing_test.go), [TestBuiltinsRunBeforeFinalRouteInterceptors](../internal/integration-tests/routing_contract_test.go) |
| Интерцепторы промежуточного route не вызываются | [TestBuiltinsRunBeforeFinalRouteInterceptors](../internal/integration-tests/routing_contract_test.go) |
| Redirect loop, явный лимит больше default, redirect без совпавшего old | [TestMethodsHostMatchingAndRedirectLoop](../internal/integration-tests/route_selection_test.go), [TestExplicitRedirectLimitCanExceedDefault](../internal/integration-tests/redirect_test.go), [TestNoMatchingInternalRedirectHasNoImplicitTarget](../internal/integration-tests/processing_errors_test.go) |
| `allowedMethods`: отказ 403 без перехода к fallback; GET разрешает HEAD | [TestMethodsHostMatchingAndRedirectLoop](../internal/integration-tests/route_selection_test.go) |
| `directResponse`, отсутствие тела на HEAD/204/304 | [TestMethodsHostMatchingAndRedirectLoop](../internal/integration-tests/route_selection_test.go), [TestResponseStatusMutationRemovesForbiddenBody](../internal/integration-tests/response_status_test.go) |
| Basic Auth SHA/bcrypt, challenge, неверный пользователь/пароль; проверка до interceptor | [TestLocalBasicAuth](../internal/integration-tests/basic_auth_test.go), [TestBcryptAuthRunsBeforeInterceptors](../internal/integration-tests/basic_auth_test.go) |
| Удаление internalHeaders перед цепочкой | [TestMixedChainSessionAuthorizationAndReplace](../internal/integration-tests/chain_test.go) |
| `addHeaders`: успешный ответ, always, приоритет над upstream, `{name}` и literals | [TestAddHeadersSuccessAlwaysLiteralAndMismatch](../internal/integration-tests/routing_contract_test.go), [TestRoutingRewriteAndProxy](../internal/integration-tests/routing_test.go) |

## Цепочка и API интерцепторов

| Возможность | Проверка |
|---|---|
| Смешанная цепочка gRPC → REST: session → authorization; следующий видит предыдущие mutations | [TestMixedChainSessionAuthorizationAndReplace](../internal/integration-tests/chain_test.go) |
| PathParameters после rewrite/redirect, configured parameters и контекст запроса | [TestBuiltinsRunBeforeFinalRouteInterceptors](../internal/integration-tests/routing_contract_test.go), [TestImmediateResponseStopsEveryPhaseAndTransport](../internal/integration-tests/phase_contract_test.go), [TestHTTPSContextAndEndOfStream](../internal/gateway/app/body_test.go) |
| Headers set/replace, append/remove, case-insensitive пересечение | [TestMixedChainSessionAuthorizationAndReplace](../internal/integration-tests/chain_test.go), [TestRequestMetadataMutationEmptyBodyAndHeaderOperations](../internal/integration-tests/request_mutation_test.go) |
| Mutation method/authority/path/rawQuery и пустое replacement body | [TestRequestMetadataMutationEmptyBodyAndHeaderOperations](../internal/integration-tests/request_mutation_test.go) |
| Request/response buffered body, binary/base64, следующий processor видит изменённое тело | [TestBufferedRequestResponseAndTrailers](../internal/integration-tests/buffered_body_test.go), [TestRESTBodyAndResponseChainSeesMutations](../internal/integration-tests/phase_contract_test.go) |
| Request/response trailers: исходные значения, mutations видны следующему и обоим peers | [TestTrailerMutationsReachNextProcessorAndBothPeers](../internal/integration-tests/trailers_test.go), [TestRESTTrailersReachNextProcessorAndPeers](../internal/integration-tests/phase_contract_test.go) |
| Response status/header mutations, запрет тела для 204/304 | [TestRESTBodyAndResponseChainSeesMutations](../internal/integration-tests/phase_contract_test.go), [TestResponseStatusMutationRemovesForbiddenBody](../internal/integration-tests/response_status_test.go) |
| Отложенные response headers, overwrite немедленным ответом, repeated Set-Cookie | [TestMixedChainSessionAuthorizationAndReplace](../internal/integration-tests/chain_test.go), [TestPendingHeadersAppliedToImmediateAndOverridden](../internal/integration-tests/processing_errors_test.go) |
| ImmediateResponse останавливает цепочки в каждой из 6 фаз на обоих транспортах | 12 сценариев [TestImmediateResponseStopsEveryPhaseAndTransport](../internal/integration-tests/phase_contract_test.go); upstream не вызывается при request-отказе |
| Response-only внешние error pages заменяют тело и видят локальные gateway errors | [TestResponseOnlyErrorPages](../internal/integration-tests/response_headers_test.go), [TestExternalErrorPagesSeeGatewayFailures](../internal/integration-tests/routing_contract_test.go) |
| Request/response/replacement/immediate size limits; ошибки чтения и закрытие body | [TestLimitsAndInvalidInterceptorResponses](../internal/integration-tests/processing_limits_test.go), [TestInvalidRequestDecisionsFailClosed](../internal/integration-tests/invalid_decisions_test.go), [TestBufferedResponseReadErrors](../internal/integration-tests/processing_errors_test.go), [TestBodyReadErrorsAndClosure](../internal/gateway/app/body_test.go) |
| Разрешённые и запрещённые типы mutations во всех фазах | 36 комбинаций [TestMutationPhaseContract](../internal/gateway/app/validation_test.go), [TestInvalidDecisionsAndRequestMetadata](../internal/gateway/app/validation_test.go) |
| Fail-closed: пустое/невалидное решение, статус/headers/metadata/framing, transport errors, malformed JSON/schema/base64 | [TestInvalidRequestDecisionsFailClosed](../internal/integration-tests/invalid_decisions_test.go), [TestRESTTransportFailuresFailClosed](../internal/integration-tests/invalid_decisions_test.go) |
| Ошибка response processor на local response без повторного вызова | [TestFailedLocalResponseProcessorIsNotRetried](../internal/integration-tests/processing_errors_test.go) |
| Body processing запрещён для HTTP/2 и Upgrade | [TestRejectedDefinitions](../internal/gateway/config/validation_test.go), [TestInvalidDecisionsAndRequestMetadata](../internal/gateway/app/validation_test.go), [TestUpgradeBodyProcessingRejected](../internal/integration-tests/processing_errors_test.go) |

Реализации session, authorization, maintenance, CSP, CSRF и error pages находятся во внешних сервисах. Здесь проверяется механизм вызова, контекст, mutations и немедленные ответы; внутренние правила этих сервисов не входят в gateway.

## Проксирование, таймауты и lifecycle

| Возможность | Проверка |
|---|---|
| Метод, точный rawQuery (включая repeated/invalid escaping), binary body, repeated headers/cookies | [TestRoutingRewriteAndProxy](../internal/integration-tests/routing_test.go), [TestRawQueryIsPreservedExactly](../internal/integration-tests/raw_query_test.go) |
| Host rewrite и disableHostRewrite | [TestGzipAndForwardedHeadersPreserved](../internal/integration-tests/streaming_contract_test.go), [TestRequestMetadataMutationEmptyBodyAndHeaderOperations](../internal/integration-tests/request_mutation_test.go) |
| Gzip без распаковки, X-Forwarded-Host/Proto/For | [TestGzipAndForwardedHeadersPreserved](../internal/integration-tests/streaming_contract_test.go) |
| Streaming request trailers без processing | [TestStreamingRequestTrailersWithoutProcessing](../internal/integration-tests/trailers_test.go) |
| SSE: первая запись доступна клиенту до завершения upstream; активный stream не idle timeout | [TestSSEFlushesBeforeUpstreamFinishes](../internal/integration-tests/streaming_contract_test.go), [TestUpstreamTimeoutsAndUnavailable](../internal/integration-tests/timeouts_test.go) |
| Upgrade: handshake и двунаправленные binary bytes | [TestUpgradeProxiesBidirectionalBytes](../internal/integration-tests/streaming_contract_test.go) |
| HTTP/2 h2c, gRPC unary/bidirectional stream, metadata, status и trailers-only error | [TestGRPCUnaryStreamingStatusesAndTrailers](../internal/integration-tests/grpc_test.go) |
| HTTPS HTTP/1 и HTTP/2 с проверкой сертификата; HTTP/2 trailers в HTTP/1 | [TestVerifiedTLSUpstreamsHTTP1AndHTTP2](../internal/gateway/infra/proxy/tls_test.go) |
| Отмена запроса достигает upstream и interceptor | [TestCancellationReachesUpstream](../internal/integration-tests/cancellation_test.go), [TestInterceptorTimeoutExplicitDisableAndCancellation](../internal/integration-tests/timeout_contract_test.go), [TestRESTTimeoutCancelsProcessor](../internal/integration-tests/processing_errors_test.go) |
| Upstream unavailable/idle timeout → 502/504 | [TestUpstreamTimeoutsAndUnavailable](../internal/integration-tests/timeouts_test.go), [TestBufferedResponseReadErrors](../internal/integration-tests/processing_errors_test.go) |
| Global timeout/idle, route overrides, `0s` явно выключает оба | [TestGlobalTimeoutRouteOverrideAndExplicitDisable](../internal/integration-tests/timeout_contract_test.go) |
| Idle начинается после завершения upload, обновляется данными ответа | [TestIdleTimeoutWaitsForUploadCompletion](../internal/integration-tests/timeout_contract_test.go), [TestUpstreamTimeoutsAndUnavailable](../internal/integration-tests/timeouts_test.go) |
| Interceptor timeout и `0s`; response processor входит в общий proxy timeout | [TestLimitsAndInvalidInterceptorResponses](../internal/integration-tests/processing_limits_test.go), [TestRESTTimeoutCancelsProcessor](../internal/integration-tests/processing_errors_test.go), [TestInterceptorTimeoutExplicitDisableAndCancellation](../internal/integration-tests/timeout_contract_test.go), [TestProxyTimeoutIncludesResponseProcessor](../internal/integration-tests/response_timeout_test.go) |
| Health/readiness до запуска, после старта/остановки, alias listener | [TestHealthReadinessAndMultipleListeners](../internal/integration-tests/health_contract_test.go), [TestReadinessAndGracefulShutdown](../internal/integration-tests/health_test.go) |
| Graceful shutdown сохраняет inflight запрос; истечение grace period принудительно закрывает его | [TestGracefulShutdownDrainsInflightRequest](../internal/integration-tests/shutdown_test.go), [TestShutdownForceClosesAfterGracePeriod](../internal/gateway/infra/server/shutdown_test.go) |
| CLI flags/commands, env config, help и ошибки; собранный executable, YAML, SIGTERM | [CommandSuite](../cmd/main_test.go), [TestExecutableStartsWithYAMLAndStopsOnSignal](../internal/integration-tests/executable_test.go) |

Отдельные регрессионные сценарии защищают владение response/trailers при замене исходного ответа: transport может дочитывать закрытое тело асинхронно. Фазы immediateResponse проверяются с `-race`. gRPC trailers-only, SSE и streaming request trailers дополнительно прошли 30 повторных race-прогонов после исправлений.


Покрытие строк дополняет эту матрицу: оно не доказывает отсутствие всех возможных ошибок. Не реализуемые возможности (`dynamicTarget`, hot reload, streaming-body mutations, management API, встроенные интерцепторы) не объявляются покрытыми.

Использованные библиотеки: [testify/suite](https://pkg.go.dev/github.com/stretchr/testify/suite), [urfave/cli/v3](https://cli.urfave.org/v3/getting-started/).

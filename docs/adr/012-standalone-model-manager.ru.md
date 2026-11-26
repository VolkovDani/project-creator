# ADR-012. Опциональный менеджер моделей на одном хосте

[English version](012-standalone-model-manager.md)

## Решение

Project Creator стартует с PostgreSQL, API и интерфейсом. Приложения на одном хосте могут обращаться к общему [Models Configurator](https://github.com/VolkovDani/models-configurator/tree/develop) по версионированному HTTP API. Исходники, контейнеры и учётные данные менеджера устанавливаются и обновляются отдельно. Базовая установка обходится без submodule и работающего провайдера.

Менеджер отвечает за транспорт модельных вызовов, закрепление артефактов, ключи провайдеров для управляемых подключений, веса и диагностику среды. Project Creator отвечает за доступ к источникам, промпты, проверку ответов, решения по классификации, историю и индексы. Существующий OpenAI-совместимый клиент обеспечивает прямое подключение и работу через менеджер. Каждая роль выбирается явно. Сбой локального профиля не переключает запрос в облако.

## Подключение установки Compose

1. Запустите менеджер, настройте хотя бы одну модель и получите её полный неизменяемый ID из `GET http://127.0.0.1:8317/v1/models`. Compose менеджера создаёт общую сеть клиентов. Из Project Creator к ней подключается только backend.
2. Добавьте в приватный `.env` Project Creator настройки нужных ролей. Для отключённой роли опустите всю группу. Модель генерации должна принимать строгую JSON Schema, а профиль эмбеддингов — возвращать 1024 компонента.

```dotenv
PROJECT_CREATOR_MODELS_NETWORK=models-configurator-clients
PROJECT_CREATOR_GENERATOR_RUNTIME=openai-compatible
PROJECT_CREATOR_GENERATOR_URL=http://models-configurator:8317/v1
PROJECT_CREATOR_GENERATOR_MODEL=<chat-profile-id>
PROJECT_CREATOR_EMBEDDING_RUNTIME=openai-compatible
PROJECT_CREATOR_EMBEDDING_URL=http://models-configurator:8317/v1
PROJECT_CREATOR_EMBEDDING_MODEL=<embedding-profile-id>
```

3. Выполните `./project-creator setup --device remote` (для перехода с локального устройства добавьте `--switch-device`). Для ручного запуска Compose укажите `-f compose.yml -f compose.models-configurator.yml`. При запуске приложения непосредственно на хосте используйте loopback вместо имени контейнерного сервиса. При прямом внешнем подключении каждой роли достаточно собственного URL, ID модели и при необходимости ключа. [Руководство установки](../setup-cli.md)

ID профиля включает имя провайдера и отпечаток, учитывающий адрес, модель, роль, локальный digest или ревизию внешнего развёртывания. При смене профиля выберите новый ID и перезапустите backend. Прежний ID менеджер отклонит. Project Creator учитывает ID в существующих профилях результатов и индекса. Ревизию внешней модели указывает оператор. Подмену весов за неизменной ревизией нельзя надёжно обнаружить. При работе через менеджер ключи остаются у него. В общую сеть клиентов подключаются только доверенные приложения на том же хосте.

## Совместимость и проверка

Существующие установки CPU, GPU и NPU могут использовать сохранённое состояние установщика, имя Compose-проекта и тома. Для новых управляемых установок пилот создаёт отдельный том Ollama. Смена провайдера сохраняет старые данные. Для отдельного OpenVINO NPU используется неизменяемый ID профиля генерации менеджера. Project Creator упорядочивает пару, сопоставляет исходники и проверяет результаты на обоих маршрутах NPU. У подключения через менеджер собственные сведения о происхождении и идентификатор кеша. Перенос классификаторов ведётся отдельно. Аппаратная приёмка относится к конкретному проверенному устройству.

Межрепозиторный тест использует две синтетические роли провайдера и настоящий бинарный файл менеджера. Задайте в `TEST_DATABASE_URL` отдельную тестовую PostgreSQL с pgvector и выполните:

```bash
# Из репозитория менеджера:
go build -o /tmp/models-configurator-test ./cmd/models-configurator
# Из репозитория Project Creator:
MODELS_CONFIGURATOR_TEST_BIN=/tmp/models-configurator-test go test -count=1 ./internal/api -run 'Test(ModelManagerWorkerIntegration|CompatibleProvidersWithoutLocalRuntime)$'
```

Чтобы проверить адаптер классификации через тот же бинарный файл менеджера, выполните `MODELS_CONFIGURATOR_TEST_BIN=/tmp/models-configurator-test go test -count=1 ./internal/recordclassifier -run '^TestManagedDecisionManagerIntegration$'`. Тест использует синтетический локальный endpoint Ollama и сравнивает обе оси ревью. Запишите обе ревизии исходников вместе с результатом.

Чтобы проверить независимую оценку классификации через тот же бинарный файл менеджера, выполните `MODELS_CONFIGURATOR_TEST_BIN=/tmp/models-configurator-test go test -count=1 ./internal/api -run '^TestModelManagerAssessmentIntegration$'`. Тест подставляет ответы среды NPU и проверяет права доступа через GraphQL, цитаты из исходного текста и повторное использование кеша.

Для проверки планирования графа процессов запустите `MODELS_CONFIGURATOR_TEST_BIN=/tmp/models-configurator-test go test -v -count=1 ./internal/api -run '^TestModelManagerProcessGraphPreflight$'`. Заглушка NPU возвращает два входных бюджета. Тест проверяет, что широкому окну требуется меньше порций из целых фрагментов, планирование заканчивается до генерации, а при исчерпании лимита ответа приложение разделяет порцию в рамках ограничения на число повторов. Зафиксируйте обе ревизии вместе с результатом.

Тест проверяет сохранённые выжимки, индексацию и поиск, анализ коллизий и изоляцию рабочих областей через API. Обычные Go-тесты используют детерминированные провайдеры и могут запускаться без репозитория менеджера.

Для прогона трёх фиксированных аппаратных примеров через клиент менеджера задайте `MANAGED_NPU_REAL_SMOKE=1`, `MANAGED_NPU_URL=http://127.0.0.1:8317/v1` и полный NPU ID в `MANAGED_NPU_MODEL`, затем выполните `go test -v -count=1 ./internal/analysis -run '^TestManagedOpenVINORealNPUSmoke$'`.

С теми же переменными выполните `go test -v -count=1 ./internal/analysis -run '^TestManagedNPURealClassificationExplanation$'`, чтобы проверить оценку и подтверждающие её цитаты на синтетической пользовательской истории.

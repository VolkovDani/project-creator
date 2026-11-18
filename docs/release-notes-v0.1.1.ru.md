# Source + Compose v0.1.1

[English version](https://github.com/VolkovDani/project-creator/blob/v0.1.1/docs/release-notes-v0.1.1.md)

Патч обновляет локальный однопользовательский MVP, который устанавливается
из исходного кода через Docker Compose. Ссылки на руководства по установке
и настройке оборудования собраны в [README](https://github.com/VolkovDani/project-creator/blob/v0.1.1/README.md).

## Изменения

- Генерация типов GraphQL для TypeScript читает локальные файлы схемы и операций.
  Сгенерированные типы и проверки сохранены, цепочка уязвимых зависимостей CLI
  удалена из lock-файла. [#151](https://github.com/VolkovDani/project-creator/issues/151)
- Если индексация уже идёт, проверка установки через Compose ждёт готовности
  нужной версии требования. Время ожидания ограничено.
  [#152](https://github.com/VolkovDani/project-creator/issues/152)
- Требование редактируется в доступном диалоговом окне. Черновик создания
  сохраняется отдельно, а ошибки сохранения и конфликты версий отображаются
  в самом окне. Кнопка «Наверх» доступна с клавиатуры и учитывает настройку
  уменьшения движения. [#145](https://github.com/VolkovDani/project-creator/issues/145)
- Анализ пар в Ollama и OpenVINO проверяет алфавит объяснения. Разрешены буквы
  русского алфавита и латинские буквы ASCII. При нарушении генерация повторяется
  один раз в оставшееся время на пару. Повторное нарушение приводит к ошибке пары.
  [#99](https://github.com/VolkovDani/project-creator/issues/99)
- Патч также включает переход на Node.js 26 и обновление зависимостей,
  инструкцию по установке с помощью ИИ и сокращённый README со ссылками на
  руководства по настройке. [#140](https://github.com/VolkovDani/project-creator/pull/140),
  [#147](https://github.com/VolkovDani/project-creator/pull/147),
  [#150](https://github.com/VolkovDani/project-creator/pull/150)

## Известные ограничения

- Проверка алфавита не гарантирует, что объяснение написано по-русски или
  что вердикт верен. Сверяйте результаты модели с исходными требованиями.
  [Руководство по анализу пар](https://github.com/VolkovDani/project-creator/blob/v0.1.1/docs/requirement-analysis.ru.md)
- Сохранение языка исходного требования при генерации выжимки остаётся открытой
  задачей. [#108](https://github.com/VolkovDani/project-creator/issues/108)
- Аппаратные проверки охватывают отдельные испытанные конфигурации. Для GPU
  и NPU нужны совместимые оборудование, драйверы и локальный runtime.
  [Задача по аппаратным проверкам](https://github.com/VolkovDani/project-creator/issues/22)

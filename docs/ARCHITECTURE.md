# Устройство и источники

## Поток

`MediaRecorder → File → штатный upload callback → черновик Mattermost → отправка пользователем → MessageHasBeenPosted → KV CAS queue → Deepgram POST /v1/listen → ответ бота в исходной ветке`.

Веб-плагин использует React самого Mattermost, собственная копия React не включена в bundle. Единственная точка записи — официальное меню вложений. Так сохраняются штатные hooks загрузки файлов, разрешения, thread root, черновик и ошибки загрузки. При смене канала или открытой ветки рекордер не прикрепляет файл; предлагает скачать запись.

Сервер на Go использует `github.com/mattermost/mattermost/server/public v0.4.4`. Нет внешней БД, FFmpeg, отдельного сервиса или доступа к публичным ссылкам файлов. Только фиксированные HTTPS endpoints Deepgram, без перенаправлений. `GET /config` отдаёт только лимиты и наличие ключа; `POST /transcribe` проверяет пользователя, автора, членство и CSRF header. Автоматическая обработка не использует привилегии пользователя для чтения чужих каналов.

Минимальная версия 10.11 выбрана как консервативная базовая линия API, не как утверждение о проверке на каждом выпуске. Bot EnsureBotUser доступен с 7.1, CAS с 5.12, CAS delete с 5.16. Произвольные ключи конфигурации и произвольные URL провайдеров не поддерживаются.

## Первичные источники

Проверены при разработке 24 сентября 2026:

- [Mattermost Web App SDK](https://docs.mattermost.com/developers/integrate/reference/webapp): registry, root component, upload method, post menu.
- [Mattermost file upload implementation](https://github.com/mattermost/mattermost/blob/master/webapp/channels/src/components/file_upload/file_upload.tsx): `item.action(this.checkPluginHooksAndUploadFiles)` — фактическая сигнатура upload callback.
- [Mattermost Server SDK](https://docs.mattermost.com/developers/integrate/reference/server): hooks, file info, posts, permissions, KV.
- [Официальный starter template](https://github.com/mattermost/mattermost-plugin-starter-template): manifest layout, серверный SDK, host React.
- [Mattermost mobile plugin limitation](https://github.com/mattermost/mattermost-mobile/issues/9833): нативный mobile не показывает веб-компоненты плагина.
- [Deepgram prerecorded API](https://developers.deepgram.com/reference/speech-to-text/listen-pre-recorded): бинарное тело, Token auth, smart_format, структура ответа.
- [Deepgram models and languages](https://developers.deepgram.com/docs/models-languages-overview): Nova-3, русский.
- [Language detection](https://developers.deepgram.com/docs/language-detection): `detect_language=true`, без одновременного `language`.
- [Deepgram regions](https://developers.deepgram.com/reference/custom-endpoints): Global и EU.
- [Deepgram MIP opt-out](https://developers.deepgram.com/docs/the-deepgram-model-improvement-partnership-program): `mip_opt_out=true`.

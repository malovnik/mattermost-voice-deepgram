export const formats = ['audio/webm;codecs=opus', 'audio/ogg;codecs=opus', 'audio/mp4'];
export function supportedMime(): string {
    return formats.find((type) => MediaRecorder.isTypeSupported(type)) || '';
}
export function extension(mime: string): string {
    if (mime.includes('mp4')) { return 'm4a'; }
    if (mime.includes('ogg')) { return 'ogg'; }
    return 'webm';
}
export function microphoneError(error: unknown): string {
    const name = (error as {name?: string})?.name;
    if (name === 'NotAllowedError' || name === 'SecurityError') { return 'Разрешите доступ к микрофону в настройках браузера или приложения и повторите запись.'; }
    if (name === 'NotFoundError') { return 'Микрофон не найден. Подключите его и повторите запись.'; }
    if (name === 'NotReadableError') { return 'Микрофон занят или недоступен. Закройте другие приложения с записью и повторите.'; }
    return 'Не удалось записать звук. Проверьте микрофон и повторите запись.';
}
export function durationLabel(seconds: number): string {
    return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
}

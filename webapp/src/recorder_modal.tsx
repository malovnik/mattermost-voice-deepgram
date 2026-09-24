import React, {useEffect, useRef, useState} from 'react';
import {durationLabel, extension, microphoneError, supportedMime} from './recorder';

export interface Settings {configured: boolean; maxRecordingSeconds: number; maxFileBytes: number}
export interface RecorderProps {
    upload: (files: File[]) => void;
    close: () => void;
    isCurrent: () => boolean;
    settings: Settings;
}

export function RecorderModal({upload, close, isCurrent, settings}: RecorderProps) {
    const [phase, setPhase] = useState<'idle' | 'permission' | 'recording' | 'ready'>('idle');
    const [seconds, setSeconds] = useState(0);
    const [error, setError] = useState('');
    const [url, setURL] = useState('');
    const [blob, setBlob] = useState<Blob>();
    const [notice, setNotice] = useState('');
    const recorder = useRef<MediaRecorder>();
    const stream = useRef<MediaStream>();
    const objectURL = useRef('');
    const alive = useRef(true);
    const timer = useRef<ReturnType<typeof setInterval>>();
    const dialog = useRef<HTMLDivElement>(null);
    const adding = useRef(false);

    function release() {
        if (timer.current) { clearInterval(timer.current); timer.current = undefined; }
        stream.current?.getTracks().forEach((track) => track.stop());
        stream.current = undefined;
    }
    function stop() {
        if (recorder.current?.state === 'recording') { recorder.current.stop(); }
        release();
    }
    function dismiss() {
        // Stop capture synchronously, without waiting for React's passive cleanup.
        alive.current = false;
        stop();
        close();
    }
    useEffect(() => {
        const previous = document.activeElement as HTMLElement | null;
        dialog.current?.focus();
        return () => {
            alive.current = false;
            if (recorder.current && recorder.current.state !== 'inactive') { recorder.current.stop(); }
            release();
            if (objectURL.current) { URL.revokeObjectURL(objectURL.current); }
            previous?.focus();
        };
    }, []);

    async function start() {
        if (phase === 'permission' || phase === 'recording') { return; }
        setError(''); setNotice('');
        if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
            setError('Для записи нужен HTTPS и современный браузер. Можно прикрепить готовый файл из диктофона.'); return;
        }
        setPhase('permission');
        let active: MediaStream | undefined;
        try {
            const mime = supportedMime();
            if (!mime) { throw new Error('unsupported'); }
            active = await navigator.mediaDevices.getUserMedia({audio: {echoCancellation: true, noiseSuppression: true}, video: false});
            if (!alive.current) { active.getTracks().forEach((track) => track.stop()); return; }
            stream.current = active;
            const rec = new MediaRecorder(active, {mimeType: mime, audioBitsPerSecond: 64000});
            recorder.current = rec;
            const chunks: BlobPart[] = [];
            let size = 0;
            rec.ondataavailable = (event) => {
                if (!event.data.size) { return; }
                chunks.push(event.data); size += event.data.size;
                if (size >= settings.maxFileBytes && rec.state === 'recording') {
                    setNotice('Достигнут лимит размера записи.'); stop();
                }
            };
            rec.onerror = () => { if (alive.current) { setError('Запись прервалась. Прослушайте сохранённый фрагмент или запишите заново.'); stop(); } };
            rec.onstop = () => {
                release();
                if (!alive.current) { return; }
                const audio = new Blob(chunks, {type: rec.mimeType || mime});
                if (objectURL.current) { URL.revokeObjectURL(objectURL.current); }
                if (!audio.size) { setError('Запись пустая. Попробуйте ещё раз.'); setPhase('idle'); return; }
                objectURL.current = URL.createObjectURL(audio);
                setURL(objectURL.current); setBlob(audio); setPhase('ready');
                if (audio.size > settings.maxFileBytes) { setError('Запись превышает лимит размера. Скачайте её или запишите более короткое сообщение.'); }
            };
            active.getAudioTracks().forEach((track) => { track.onended = () => { if (rec.state === 'recording') { setNotice('Микрофон отключён. Сохранён записанный фрагмент.'); stop(); } }; });
            rec.start(250);
            setSeconds(0); setBlob(undefined); setPhase('recording');
            const began = Date.now();
            timer.current = setInterval(() => {
                const elapsed = Math.floor((Date.now() - began) / 1000);
                setSeconds(elapsed);
                if (elapsed >= settings.maxRecordingSeconds) { setNotice('Достигнута максимальная длительность.'); stop(); }
            }, 250);
        } catch (err) {
            active?.getTracks().forEach((track) => track.stop());
            release();
            if (alive.current) { setError(microphoneError(err)); setPhase(blob ? 'ready' : 'idle'); }
        }
    }

    function attach() {
        if (!blob || adding.current) { return; }
        if (!isCurrent()) { setError('Канал или ветка изменились. Скачайте запись и прикрепите её к нужному сообщению.'); return; }
        adding.current = true;
        try {
            upload([new File([blob], `voice-message-${Date.now()}.${extension(blob.type)}`, {type: blob.type})]);
            close();
        } catch {
            adding.current = false;
            setError('Не удалось добавить запись. Скачайте её, чтобы не потерять.');
        }
    }
    function keydown(event: React.KeyboardEvent) {
        if (event.key === 'Escape') { event.preventDefault(); dismiss(); }
        if (event.key !== 'Tab') { return; }
        const elements = dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled), a[href], audio, [tabindex="0"]');
        if (!elements?.length) { return; }
        const first = elements[0], last = elements[elements.length - 1];
        if (event.shiftKey && (document.activeElement === first || document.activeElement === dialog.current)) { event.preventDefault(); last.focus(); }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    }
    return <div className='vd-overlay'>
        <div ref={dialog} role='dialog' aria-modal='true' aria-labelledby='vd-title' tabIndex={-1} className='vd-dialog' onKeyDown={keydown}>
            <div className='vd-header'><h2 id='vd-title'>Голосовое сообщение</h2><button aria-label='Закрыть и удалить запись' onClick={dismiss}>×</button></div>
            <p className='vd-muted'>Запишите, прослушайте и добавьте аудио к сообщению.</p>
            {!settings.configured && <p className='vd-notice'>Запись работает. Расшифровка появится после подключения ключа Deepgram администратором и повторного запроса.</p>}
            <div className={'vd-clock ' + (phase === 'recording' ? 'vd-live' : '')} aria-live='off'>{durationLabel(seconds)}</div>
            <p className='vd-muted'>{phase === 'recording' ? 'Идёт запись' : phase === 'permission' ? 'Ожидаем разрешение на микрофон…' : `До ${durationLabel(settings.maxRecordingSeconds)}`}</p>
            {phase === 'ready' && url && <audio controls src={url} aria-label='Прослушать запись'/>}
            {notice && <p role='status'>{notice}</p>}
            {error && <p className='vd-error' role='alert'>{error}</p>}
            <div className='vd-actions'>
                {phase === 'recording' ? <button className='vd-primary' onClick={stop}>Остановить</button> : <button disabled={phase === 'permission'} onClick={start}>{phase === 'ready' ? 'Записать заново' : 'Начать запись'}</button>}
                {phase === 'ready' && blob && <button className='vd-primary' disabled={blob.size > settings.maxFileBytes} onClick={attach}>Добавить к сообщению</button>}
            </div>
            {phase === 'ready' && blob && <a href={url} download={`voice-message.${extension(blob.type)}`}>Скачать запись</a>}
            <p className='vd-footnote'>После добавления нажмите «Отправить» в Mattermost. Опубликованное аудио будет отправлено в Deepgram для расшифровки. Закрытие окна удалит локальную запись.</p>
        </div>
    </div>;
}

import React from 'react';
import {RecorderModal, RecorderProps, Settings} from './recorder_modal';
import {styles} from './styles';

const ID = 'com.malovnik.voice-deepgram';
type Store = {getState: () => any};
type Upload = (files: File[]) => void;
interface Registry {
    registerRootComponent: (component: React.ComponentType) => string;
    registerFileUploadMethod: (icon: React.ReactNode, action: (upload: Upload) => void, text: string) => string;
    registerPostDropdownMenuAction: (text: string, action: (postId: string) => void, filter: (postId: string) => boolean) => string;
}
const mic = <svg width='20' height='20' viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='1.8' aria-hidden='true'><rect x='8' y='2' width='8' height='13' rx='4'/><path d='M5 10v2a7 7 0 0014 0v-2M12 19v3M8 22h8'/></svg>;

export function basePath(store: Store): string {
    const site = store.getState().entities?.general?.config?.SiteURL;
    return site ? new URL(site, window.location.origin).pathname.replace(/\/$/, '') : (window.basename || '');
}
function contextKey(store: Store) {
    const s = store.getState();
    return JSON.stringify([s.entities?.channels?.currentChannelId, s.views?.rhs?.selectedPostId]);
}

export class Plugin {
    private show?: (props?: RecorderProps, message?: string) => void;
    private active = false;
    private opening = false;
    initialize(registry: Registry, store: Store) {
        this.active = true;
        const plugin = this;
        function Root() {
            const [props, setProps] = React.useState<RecorderProps>();
            const [message, setMessage] = React.useState('');
            React.useEffect(() => { plugin.show = (next, text = '') => { setProps(next); setMessage(text); }; return () => { plugin.show = undefined; }; }, []);
            return <><style>{styles}</style>{props && <RecorderModal {...props}/>} {message && <div className='vd-toast' role='status'>{message}<button aria-label='Закрыть' onClick={() => setMessage('')}>×</button></div>}</>;
        }
        registry.registerRootComponent(Root);
        registry.registerFileUploadMethod(mic, async (upload) => {
            if (this.opening || !this.active) { return; }
            this.opening = true;
            const original = contextKey(store);
            try {
                const response = await fetch(`${basePath(store)}/plugins/${ID}/config`, {credentials:'same-origin'});
                if (!response.ok) { throw new Error(); }
                const settings = await response.json() as Settings;
                if (!this.active) { return; }
                this.show?.({upload, settings, close:() => this.show?.(), isCurrent:() => contextKey(store) === original});
            } catch { this.show?.(undefined, 'Не удалось загрузить настройки записи. Обновите страницу и попробуйте ещё раз.'); }
            finally { this.opening = false; }
        }, 'Голосовое сообщение');
        registry.registerPostDropdownMenuAction('Расшифровать аудио', async (postId) => {
            try {
                const csrf = document.cookie.match(/(?:^|;\s*)MMCSRF=([^;]+)/)?.[1] || '';
                const response = await fetch(`${basePath(store)}/plugins/${ID}/transcribe`, {method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-Requested-With':'XMLHttpRequest','X-CSRF-Token':csrf},body:JSON.stringify({post_id:postId})});
                const result = await response.json();
                this.show?.(undefined, response.ok ? 'Запрос принят. Расшифровка появится в ветке сообщения.' : result.error || 'Не удалось запустить расшифровку.');
            } catch { this.show?.(undefined, 'Нет связи с сервером. Повторите запрос позже.'); }
        }, (postId) => {
            const s = store.getState();
            const post = s.entities?.posts?.posts?.[postId];
            return Boolean(post?.file_ids?.length && post.user_id === s.entities?.users?.currentUserId);
        });
    }
    uninitialize() { this.active = false; this.show?.(); this.show = undefined; }
}
declare global { interface Window {registerPlugin: (id: string, plugin: Plugin) => void; basename?: string} }
window.registerPlugin(ID, new Plugin());

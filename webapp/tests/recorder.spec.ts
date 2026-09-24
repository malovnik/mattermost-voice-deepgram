import {test,expect,Page} from '@playwright/test';

async function open(page:Page,config={configured:true,maxRecordingSeconds:300,maxFileBytes:25*1024*1024}) {
    await page.route('**/plugins/com.malovnik.voice-deepgram/config',route=>route.fulfill({json:config}));
    await page.goto('/');
    await page.getByRole('button',{name:'Голосовое сообщение',exact:true}).click();
    await expect(page.getByRole('dialog')).toBeVisible();
}
async function record(page:Page) {
    await page.getByRole('button',{name:'Начать запись'}).click();
    await expect(page.getByText('Идёт запись',{exact:true})).toBeVisible();
    await expect(page.locator('.vd-clock')).toHaveText('0:01');
    await page.getByRole('button',{name:'Остановить',exact:true}).click();
    await expect(page.getByRole('button',{name:'Добавить к сообщению'})).toBeVisible();
}
test('real MediaRecorder makes playable WebM and hands a File to native composer',async({page})=>{
    await open(page);await record(page);
    await page.screenshot({path:'test-results/recorder.png'});
    const audio=await page.locator('audio').evaluate(async(el:HTMLAudioElement)=>{await el.play();el.pause();return {ready:el.readyState,error:el.error?.message};});
    expect(audio.error).toBeUndefined();expect(audio.ready).toBeGreaterThan(0);
    await page.getByRole('button',{name:'Добавить к сообщению'}).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    const result=await page.evaluate(async()=>{const file=(window as any).uploaded[0] as File;return {name:file.name,size:file.size,type:file.type,magic:Array.from(new Uint8Array(await file.slice(0,4).arrayBuffer()))};});
    expect(result.name).toMatch(/^voice-message-\d+\.webm$/);expect(result.size).toBeGreaterThan(100);expect(result.magic).toEqual([26,69,223,163]);
});
test('closing stops microphone and never uploads',async({page})=>{
    await open(page);
    await page.evaluate(()=>{const original=navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);navigator.mediaDevices.getUserMedia=async(c)=>{const stream=await original(c);(window as any).stream=stream;return stream;};});
    await page.getByRole('button',{name:'Начать запись'}).click();await expect(page.getByText('Идёт запись',{exact:true})).toBeVisible();
    await page.getByRole('button',{name:'Закрыть и удалить запись'}).click();
    expect(await page.evaluate(()=>(window as any).stream.getTracks().every((track:MediaStreamTrack)=>track.readyState==='ended'))).toBe(true);
    expect(await page.evaluate(()=>(window as any).uploaded)).toBeUndefined();
});
test('permission denial is actionable and retryable',async({page})=>{
    await open(page);await page.evaluate(()=>{navigator.mediaDevices.getUserMedia=async()=>{throw new DOMException('denied','NotAllowedError');};});
    await page.getByRole('button',{name:'Начать запись'}).click();await expect(page.getByRole('alert')).toContainText('Разрешите доступ');await expect(page.getByRole('button',{name:'Начать запись'})).toBeEnabled();
});
test('channel switch preserves local file and blocks wrong destination',async({page})=>{
    await open(page);await record(page);await page.evaluate(()=>{(window as any).testState.entities.channels.currentChannelId='channel-b';});
    await page.getByRole('button',{name:'Добавить к сообщению'}).click();await expect(page.getByRole('alert')).toContainText('Канал или ветка изменились');expect(await page.evaluate(()=>(window as any).uploaded)).toBeUndefined();await expect(page.getByRole('link',{name:'Скачать запись'})).toBeVisible();
});
test('duration cap stops recording and allows review without a provider key',async({page})=>{
    await open(page,{configured:false,maxRecordingSeconds:1,maxFileBytes:25*1024*1024});await expect(page.getByText('Запись работает.',{exact:false})).toBeVisible();
    await page.getByRole('button',{name:'Начать запись'}).click();await expect(page.getByRole('button',{name:'Добавить к сообщению'})).toBeVisible();await expect(page.getByRole('status')).toContainText('максимальная длительность');
});
test('late microphone permission is cleaned after modal is closed',async({page})=>{
    await open(page);await page.evaluate(()=>{const original=navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);navigator.mediaDevices.getUserMedia=async(c)=>{const stream=await original(c);(window as any).lateStream=stream;await new Promise<void>(resolve=>{(window as any).resolvePermission=resolve;});return stream;};});
    await page.getByRole('button',{name:'Начать запись'}).click();await page.waitForFunction(()=>(window as any).resolvePermission);
    await page.getByRole('button',{name:'Закрыть и удалить запись'}).click();await page.evaluate(()=>(window as any).resolvePermission());
    await expect.poll(()=>page.evaluate(()=>(window as any).lateStream.getTracks().every((t:MediaStreamTrack)=>t.readyState==='ended'))).toBe(true);
});
test('subpath installation calls plugin API below SiteURL path',async({page})=>{
    let requested=false;await page.route('**/teamchat/plugins/**/config',route=>{requested=true;return route.fulfill({json:{configured:true,maxRecordingSeconds:300,maxFileBytes:1024*1024}});});
    await page.goto('/');await page.evaluate(()=>{(window as any).testState.entities.general.config.SiteURL=location.origin+'/teamchat';});await page.getByRole('button',{name:'Голосовое сообщение',exact:true}).click();await expect(page.getByRole('dialog')).toBeVisible();expect(requested).toBe(true);
});

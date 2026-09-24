// Runs only against the disposable CI instance. Never point this at a production server.
import {readFileSync} from 'node:fs';
import {randomUUID} from 'node:crypto';
import assert from 'node:assert/strict';
const base='http://localhost:8065';
const manifest=JSON.parse(readFileSync('plugin.json','utf8'));
let token='';
async function api(path,method='GET',body,form=false){
 const headers={};if(token){headers.Authorization=`Bearer ${token}`;}if(body&&!form){headers['Content-Type']='application/json';}
 const res=await fetch(base+path,{method,headers,body:body?(form?body:JSON.stringify(body)):undefined});
 const data=await res.json();
 assert(res.ok,`${method} ${path}: ${res.status} ${data.message||''}`);
 return {data,res};
}
const password='Smoke-'+randomUUID()+'!aA1';
const {data:user}=await api('/api/v4/users','POST',{email:'voice-smoke@example.test',username:'voice-smoke',password});
const login=await api('/api/v4/users/login','POST',{login_id:user.username,password});token=login.res.headers.get('Token');assert(token);
const {data:team}=await api('/api/v4/teams','POST',{name:'voice-test',display_name:'Voice test',type:'O'});
const {data:channel}=await api('/api/v4/channels','POST',{team_id:team.id,name:'voice-test',display_name:'Voice test',type:'O'});
const form=new FormData();form.set('plugin',new Blob([readFileSync(`dist/${manifest.id}-${manifest.version}.tar.gz`)]),'plugin.tar.gz');
const {data:installed}=await api('/api/v4/plugins','POST',form,true);assert.equal(installed.id,manifest.id);
await api(`/api/v4/plugins/${manifest.id}/enable`,'POST');
const {data:config}=await api(`/plugins/${manifest.id}/config`);assert.equal(config.configured,false);assert.equal(config.maxRecordingSeconds,300);assert(!JSON.stringify(config).includes('DeepgramAPIKey'));
const unauth=await fetch(`${base}/plugins/${manifest.id}/config`);assert.equal(unauth.status,401);
const {data:plugins}=await api('/api/v4/plugins');assert(plugins.active.some(p=>p.id===manifest.id));
const {data:commands}=await api(`/api/v4/commands?team_id=${team.id}`);assert(commands.some(c=>c.trigger==='voice'));
const {data:bots}=await api('/api/v4/bots');assert(bots.some(b=>b.username==='voice-transcriber'));
const wave=Buffer.alloc(44+3200);wave.write('RIFF',0);wave.writeUInt32LE(wave.length-8,4);wave.write('WAVEfmt ',8);wave.writeUInt32LE(16,16);wave.writeUInt16LE(1,20);wave.writeUInt16LE(1,22);wave.writeUInt32LE(16000,24);wave.writeUInt32LE(32000,28);wave.writeUInt16LE(2,32);wave.writeUInt16LE(16,34);wave.write('data',36);wave.writeUInt32LE(3200,40);
const upload=new FormData();upload.set('channel_id',channel.id);upload.set('files',new Blob([wave],{type:'audio/wav'}),'voice-message-smoke.wav');
const {data:files}=await api('/api/v4/files','POST',upload,true);
const {data:post}=await api('/api/v4/posts','POST',{channel_id:channel.id,message:'Audio works without a provider key',file_ids:[files.file_infos[0].id]});
const {data:info}=await api(`/api/v4/files/${files.file_infos[0].id}/info`);assert.equal(info.post_id,post.id);
const file=await fetch(`${base}/api/v4/files/${info.id}`,{headers:{Authorization:`Bearer ${token}`}});assert.equal((await file.arrayBuffer()).byteLength,wave.length);
const {data:command}=await api('/api/v4/commands/execute','POST',{channel_id:channel.id,command:`/voice transcribe ${post.id}`});assert(command.response.text.includes('ключ Deepgram'));
console.log('PASS: Mattermost 10.11 installation, activation, bot, command, auth, secret-free config, native upload/post/download, no-key behavior.');

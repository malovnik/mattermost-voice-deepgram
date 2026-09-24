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
const {data:command}=await api('/api/v4/commands/execute','POST',{channel_id:channel.id,command:`/voice transcribe ${post.id}`});assert(command.text.includes('ключ Deepgram'));
console.log('PASS: Mattermost 10.11 installation, activation, bot, command, auth, secret-free config, native upload/post/download, no-key behavior.');

// Connect to an isolated TLS stub using the production hostname and real binary.
// The CI Docker network resolves this name locally. No audio or key reaches Deepgram.
await api('/api/v4/config/patch','PUT',{PluginSettings:{Plugins:{[manifest.id]:{deepgramapikey:'ci-test-only'}}}});
async function replyFor(source,status='done'){
 for(let i=0;i<45;i++){
  const {data:thread}=await api(`/api/v4/posts/${source.id}/thread`);
  const reply=Object.values(thread.posts).find(p=>p.props?.voice_source_post_id===source.id&&p.props?.voice_status===status);
  if(reply){return reply;}await new Promise(resolve=>setTimeout(resolve,1000));
 }
 throw new Error(`No ${status} reply for test post`);
}
async function sendAudio(destination,root=''){
 const data=new FormData();data.set('channel_id',destination);data.set('files',new Blob([wave],{type:'audio/wav'}),'voice-message-smoke.wav');
 const upload=await api('/api/v4/files','POST',data,true);
 return (await api('/api/v4/posts','POST',{channel_id:destination,root_id:root,message:'CI voice test',file_ids:[upload.data.file_infos[0].id]})).data;
}
const automatic=await sendAudio(channel.id);
const transcript=await replyFor(automatic);assert.equal(transcript.root_id,automatic.id);assert(transcript.message.includes('Привет, команда'));assert(!transcript.message.includes('@channel'));
const nested=await sendAudio(channel.id,automatic.id);const nestedReply=await replyFor(nested);assert.equal(nestedReply.root_id,automatic.id);
const {data:other}=await api('/api/v4/users','POST',{email:'voice-other@example.test',username:'voice-other',password:'Other-'+randomUUID()+'!aA1'});
const {data:dm}=await api('/api/v4/channels/direct','POST',[user.id,other.id]);const dmPost=await sendAudio(dm.id);assert.equal((await replyFor(dmPost)).channel_id,dm.id);
const stats=await(await fetch('http://localhost:8090')).json();assert.equal(stats.calls,3);assert.deepEqual(stats.errors,[]);
await api('/api/v4/commands/execute','POST',{channel_id:channel.id,command:`/voice transcribe ${automatic.id}`});
await new Promise(resolve=>setTimeout(resolve,1500));assert.equal((await(await fetch('http://localhost:8090')).json()).calls,3);
await fetch('http://localhost:8090',{method:'POST',body:JSON.stringify({failure:401})});
const failed=await sendAudio(channel.id);const failureReply=await replyFor(failed,'failed');assert(!failureReply.message.includes('private provider detail'));
await fetch('http://localhost:8090',{method:'POST',body:JSON.stringify({failure:0})});
await api('/api/v4/commands/execute','POST',{channel_id:channel.id,command:`/voice transcribe ${failed.id}`});
const retried=await replyFor(failed);assert.equal(retried.id,failureReply.id);
console.log('PASS: Real Mattermost hook → durable queue → HTTPS Deepgram contract stub → bot reply, DM/thread routing, deduplication, 401 failure and manual recovery.');

// CI only: Docker DNS maps api.deepgram.com to this TLS contract stub.
import https from 'node:https';
import http from 'node:http';
import {readFileSync} from 'node:fs';
let calls=0;
let failure=0;
const errors=[];
https.createServer({key:readFileSync('/cert/key.pem'),cert:readFileSync('/cert/cert.pem')},async(req,res)=>{
 const chunks=[];for await(const chunk of req){chunks.push(chunk);}const body=Buffer.concat(chunks);
 const u=new URL(req.url,'https://api.deepgram.com');
 if(req.method!=='POST'||u.pathname!=='/v1/listen'||u.searchParams.get('language')!=='ru'||u.searchParams.get('mip_opt_out')!=='true'||req.headers.authorization!=='Token ci-test-only'||req.headers['content-type']!=='audio/wav'||body.toString('ascii',0,4)!=='RIFF'){errors.push('Invalid request contract');res.writeHead(400);res.end('{}');return;}
 calls++;res.setHeader('Content-Type','application/json');
 if(failure){res.writeHead(failure);res.end('{"err_msg":"deliberately private provider detail"}');return;}
 res.end(JSON.stringify({results:{channels:[{alternatives:[{transcript:'Привет, команда! Проверка @channel и [ссылки](https://example.com).'}]}]}}));
}).listen(443,'0.0.0.0');
http.createServer(async(req,res)=>{
 if(req.method==='POST'){const chunks=[];for await(const chunk of req){chunks.push(chunk);}failure=JSON.parse(Buffer.concat(chunks).toString()).failure;}
 res.setHeader('Content-Type','application/json');res.end(JSON.stringify({calls,errors,failure}));
}).listen(8090,'0.0.0.0');

import http from 'node:http';
import {readFile} from 'node:fs/promises';
const files = {'/':'tests/harness.html','/react.js':'node_modules/react/umd/react.development.js','/react-dom.js':'node_modules/react-dom/umd/react-dom.development.js','/plugin.js':'dist/main.js'};
http.createServer(async (req,res) => {
  const file=files[req.url];
  if(!file){res.writeHead(404);res.end();return;}
  res.setHeader('Content-Type',file.endsWith('.html')?'text/html':'application/javascript');
  res.end(await readFile(file));
}).listen(8176,'127.0.0.1');

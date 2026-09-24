import {defineConfig} from '@playwright/test';
export default defineConfig({testDir:'./tests',timeout:20000,workers:1,use:{baseURL:'http://127.0.0.1:8176',headless:true,launchOptions:{args:['--use-fake-ui-for-media-stream','--use-fake-device-for-media-stream']},permissions:['microphone']},webServer:{command:'node tests/serve.mjs',url:'http://127.0.0.1:8176',reuseExistingServer:false}});

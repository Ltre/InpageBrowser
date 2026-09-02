package server

import (
	"io"
	"net/http"
)

const appJS = `
const $ = (s) => document.querySelector(s);
const b64url = (buf) => { const bytes = new Uint8Array(buf); let s = ''; bytes.forEach(b => s += String.fromCharCode(b)); return btoa(s).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,''); };
const from64 = (s) => { s=s.replace(/-/g,'+').replace(/_/g,'/'); while(s.length%4)s+='='; const raw=atob(s); return Uint8Array.from(raw,c=>c.charCodeAt(0)); };
async function api(path, body) {
  const r = await fetch(path,{method:body===undefined?'GET':'POST',credentials:'same-origin',headers:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});
  const data = await r.json().catch(()=>({}));
  if(!r.ok) throw new Error(data.error||('HTTP '+r.status));
  return data;
}
function credentialJSON(c, tx) {
  return {tx,id:c.id,rawId:b64url(c.rawId),type:c.type,response:{clientDataJSON:b64url(c.response.clientDataJSON),attestationObject:c.response.attestationObject?b64url(c.response.attestationObject):'',authenticatorData:c.response.authenticatorData?b64url(c.response.authenticatorData):'',signature:c.response.signature?b64url(c.response.signature):''}};
}
async function register() {
  try {
    const x=await api('/api/register/start',{phone:$('#phone').value});
    const p=x.publicKey; p.challenge=from64(p.challenge); p.user.id=from64(p.user.id);
    const c=await navigator.credentials.create({publicKey:p});
    await api('/api/register/finish',credentialJSON(c,x.tx)); location.href='/';
  } catch(e){ $('#error').textContent=e.message; }
}
async function login() {
  try {
    const x=await api('/api/login/start',{phone:$('#phone').value});
    const p=x.publicKey; p.challenge=from64(p.challenge); p.allowCredentials=(p.allowCredentials||[]).map(v=>({...v,id:from64(v.id)}));
    const c=await navigator.credentials.get({publicKey:p});
    await api('/api/login/finish',credentialJSON(c,x.tx)); location.href='/';
  } catch(e){ $('#error').textContent=e.message; }
}
if($('#register')) $('#register').onclick=register;
if($('#login')) $('#login').onclick=login;

const frame=$('#browser-frame'), bar=$('#address');
if(frame){
  let ready=false, editing=false;
  const setHint=(text)=>{bar.placeholder=text||'输入网址或搜索内容'};
  const start=async()=>{
    try{
      bar.disabled=true; setHint('正在启动远程 Chromium…');
      await api('/api/runtime/start',{});
      frame.src='/runtime/'; ready=true; bar.disabled=false; setHint('输入网址或搜索内容'); bar.focus();
      setInterval(()=>api('/api/runtime/heartbeat',{}).catch(()=>{}),30000);
      setInterval(syncURL,2000);
    }catch(e){bar.disabled=false;setHint(e.message);}
  };
  const syncURL=async()=>{if(!ready||editing9return;try{const x=await api('/api/runtime/current');if(x.url&&x.url!=='about:blank')bar.value=x.url;}catch(_){}};
  bar.addEventListener('focus',()=>editing=true); bar.addEventListener('blur',()=>editing=false);
  $('#address-form').addEventListener('submit',async(e)=>{
    e.preventDefault(); if(!ready)return;
    try{bar.disabled=true;const x=await api('/api/runtime/navigate',{url:bar.value});bar.value=x.url;}catch(err){setHint(err.message);}finally{bar.disabled=false;bar.blur();}
  });
  start();
}
`

func staticJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = io.WriteString(w, appJS)
}

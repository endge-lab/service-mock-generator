#!/usr/bin/env python3
"""Isolated real TCP gRPC/SSE harness; uses local Keycloak, never accesses a database.
Build both test/harness executables (see README) and pass --mock-bin/--backend-bin.
Client identity is a synthetic fixture; service identity uses real client_credentials.
"""
import argparse, hashlib, concurrent.futures, http.client, json, os, pathlib, random, socket, statistics, subprocess, threading, time, urllib.request
P=argparse.ArgumentParser();P.add_argument('--mock-bin',required=True);P.add_argument('--backend-bin',required=True);P.add_argument('--duration',type=int,default=900);P.add_argument('--cycles',type=int,default=1000);P.add_argument('--skip-lease',action='store_true');P.add_argument('--output',required=True);a=P.parse_args()
root=pathlib.Path(__file__).resolve().parents[4]
realm=json.loads((root/'infra/keycloak/realm/endge-local-realm.json').read_text())
client=next(c for c in realm['clients'] if c['clientId']=='endge-service-backend')
def freeport():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
grpcport,statsport,httpport=freeport(),freeport(),freeport()
env=os.environ.copy();env.update(HARNESS_GRPC_PORT=str(grpcport),HARNESS_STATS_PORT=str(statsport),HARNESS_HTTP_PORT=str(httpport),HARNESS_ISSUER='http://localhost:8180/realms/endge-local',HARNESS_JWKS='http://localhost:8180/realms/endge-local/protocol/openid-connect/certs',HARNESS_TOKEN_URL='http://localhost:8180/realms/endge-local/protocol/openid-connect/token',HARNESS_CLIENT_SECRET=client['secret'])
report={'checks':[],'samples':[],'latenciesMs':[],'errors':[],'binariesSha256':{'mock':hashlib.sha256(pathlib.Path(a.mock_bin).read_bytes()).hexdigest(),'backend':hashlib.sha256(pathlib.Path(a.backend_bin).read_bytes()).hexdigest()}};procs={};logs={};out=pathlib.Path(a.output);out.parent.mkdir(parents=True,exist_ok=True)
connections={};connection_lock=threading.Lock();pool=concurrent.futures.ThreadPoolExecutor(max_workers=4)
def close_connections():
 with connection_lock:
  for c in connections.values():c.close()
  connections.clear()
def start(name):
 logs[name]=open(str(out)+'.'+name+'.log','a');procs[name]=subprocess.Popen([a.mock_bin if name=='mock' else a.backend_bin],env=env,stdout=logs[name],stderr=subprocess.STDOUT)
 port=statsport if name=='mock' else httpport
 for _ in range(100):
  if procs[name].poll() is not None:raise AssertionError(name+' exited during startup')
  try:
   with urllib.request.urlopen(f'http://127.0.0.1:{port}/stats',timeout=1) as r:json.load(r)
   return
  except Exception:time.sleep(.1)
 raise AssertionError(name+' startup timeout')
def stop(name):
 if name=='backend':close_connections()
 p=procs.pop(name);p.terminate()
 try:p.wait(12)
 except subprocess.TimeoutExpired:p.kill();p.wait()
 logs.pop(name).close()
def request(method,path,data=None,actor='user-0',workspace='workspace-0',expected=(200,),role='viewer'):
 with connection_lock:
  key=threading.get_ident()
  if key not in connections:connections[key]=http.client.HTTPConnection('127.0.0.1',httpport,timeout=15)
  c=connections[key]
 body=None if data is None else json.dumps(data);t=time.monotonic();c.request(method,'/api/v1/mock-data'+path,body,{'Content-Type':'application/json','X-Test-Actor':actor,'X-Endge-Workspace':workspace,'X-Test-Role':role});r=c.getresponse();raw=r.read();report['latenciesMs'].append(round((time.monotonic()-t)*1000,3));assert r.status in expected,(method,path,r.status,raw[:300]);return json.loads(raw) if raw else None
schema={'$schema':'https://json-schema.org/draft/2020-12/schema','type':'object','properties':{'id':{'type':'integer','minimum':0,'maximum':9999},'name':{'type':'string','pattern':'^row[0-9]{4}$'}},'required':['id','name'],'additionalProperties':False}
def create(actor='user-0',custom=None):return request('POST','/streams',custom or {'schema':schema,'generation':{'seed':'stable'},'stream':{'intervalMs':100,'itemsPerMessage':3,'emitImmediately':True}},actor=actor,expected=(201,))
def subscribe(info,actor='user-0'):
 c=http.client.HTTPConnection('127.0.0.1',httpport,timeout=15);c.request('GET',info['eventsUrl'],headers={'X-Test-Actor':actor,'X-Endge-Workspace':'workspace-0'});r=c.getresponse();assert r.status==200,(r.status,r.read()[:300]);assert 'text/event-stream' in r.getheader('content-type');return c,r
# Depending on DTO spelling, accept only the actual public URL field after first response.
def event(r):
 while True:
  line=r.readline()
  if not line:return None
  if line.startswith(b'data: '):
   x=json.loads(line[6:]);assert x['type'] in ('started','data','completed','failed')
   if x['type']=='data':
    for item in x['items']:
     assert set(item)=={'id','name'} and type(item['id']) is int and 0<=item['id']<=9999 and len(item['name'])==7 and item['name'].startswith('row') and item['name'][3:].isdigit(),item
   return x
def sample():
 entry={'seconds':round(time.monotonic()-begun,2)}
 for name,port in [('mock',statsport),('backend',httpport)]:
  assert procs[name].poll() is None,name+' unexpected process exit'
  with urllib.request.urlopen(f'http://127.0.0.1:{port}/stats',timeout=3) as r:entry[name]=json.load(r)
  entry[name]['rssKiB']=int(subprocess.check_output(['ps','-o','rss=','-p',str(procs[name].pid)],text=True).strip())
 assert entry['mock']['stream']['sessions']<=32 and entry['backend']['sessions']<=32
 assert entry['mock']['stream']['jobs']<=4 and 0<=entry['mock']['stream']['bufferedBytes']<=64<<20
 report['samples'].append(entry);pathlib.Path(str(out)+'.progress.json').write_text(json.dumps(entry));return entry
def cleanup(timeout=20):
 end=time.monotonic()+timeout
 while time.monotonic()<end:
  x=sample()
  if x['mock']['stream']['sessions']==x['backend']['sessions']==0 and x['mock']['stream']['jobs']==0 and x['mock']['stream']['bufferedBytes']==x['backend']['bufferedBytes']==0:return x
  time.sleep(.3)
 raise AssertionError('cleanup did not return to zero')
def cycle(i):
 actor='cycle-'+str(i%4);info=create(actor);c,r=subscribe(info,actor)
 try:
  started=event(r);data=event(r);assert started['type']=='started' and started['streamId']==info['id'];assert data['sequence']==1 and data['streamId']==info['id'] and len(data['items'])==3
  request('PATCH','/streams/'+info['id'],{'paused':True},actor=actor)
  request('POST','/streams/'+info['id']+'/keepalive',actor=actor)
  request('PATCH','/streams/'+info['id'],{'paused':False,'itemsPerMessage':2},actor=actor)
  request('DELETE','/streams/'+info['id'],actor=actor,expected=(204,))
 finally:r.close();c.close()
def stacks(label):
 for name,port in [('mock',statsport),('backend',httpport)]:
  with urllib.request.urlopen(f'http://127.0.0.1:{port}/goroutines',timeout=3) as r:pathlib.Path(str(out)+'.'+name+'.'+label+'.stacks').write_bytes(r.read())
def mark(name):report['checks'].append(name);print(name,flush=True)
begun=time.monotonic()
try:
 start('mock');start('backend');caps=request('GET','/capabilities');assert caps['available'] and caps['canRun'],caps
 info=create();print('stream response fields: '+','.join(info.keys()),flush=True)
 # Public API uses eventsUrl; schema-preserving relay must not expose upstream ID.
 c,r=subscribe(info);assert event(r)['streamId']==info['id'];assert event(r)['sequence']==1
 request('GET','/streams/'+info['id'],actor='foreign',expected=(404,));request('GET','/streams/'+info['id'],workspace='foreign',expected=(404,));request('GET','/streams/'+info['id'],role='guest',expected=(403,));request('GET','/streams/'+info['id']+'/events',expected=(409,));request('PATCH','/streams/'+info['id'],{'schema':{}},expected=(400,));request('DELETE','/streams/'+info['id'],expected=(204,));r.close();c.close();cleanup();mark('ownership, RBAC, double subscribe, strict patch, stop')
 sessions=[create() for _ in range(5)];request('POST','/streams',{'schema':schema,'generation':{},'stream':{'intervalMs':100,'itemsPerMessage':1}},expected=(429,))
 for i in sessions:request('DELETE','/streams/'+i['id'],expected=(204,))
 cleanup();mark('per-user admission quota')
 for invalid in [{'type':'object'}, {'$schema':schema['$schema'],'type':'string','pattern':'(a+)+$'}, {'$schema':schema['$schema'],'$ref':'https://example.invalid/schema'}, {'$schema':schema['$schema'],'type':'array','minItems':4,'uniqueItems':True,'items':{'type':'boolean'}}]:request('POST','/generate',{'schema':invalid,'generation':{'count':1}},expected=(400,413,429))
 mark('invalid and impossible schemas rejected')
 list(pool.map(cycle,range(a.cycles)))
 baseline=cleanup();mark(str(a.cycles)+' create/subscribe/update/keepalive/stop cycles')
 if not a.skip_lease:
  # Real TTL: data continues throughout 180 seconds; data and heartbeat never renew lease.
  info=create();c,r=subscribe(info);assert event(r)['type']=='started';lease_start=time.monotonic();count=0
  while True:
   e=event(r)
   if not e or e['type'] in ('completed','failed'):break
   count+=1
  elapsed=time.monotonic()-lease_start;report["leaseElapsedSeconds"]=round(elapsed,3);report["leaseDataBatches"]=count;assert 178<=elapsed<=195,(elapsed,count);r.close();c.close();cleanup();mark('real 180-second lease expired while data continued')
 # Global quota counts all actors, independently of per-user quotas.
 all_sessions=[]
 for i in range(32):all_sessions.append((str(i),create('global-'+str(i))))
 request('POST','/streams',{'schema':schema,'generation':{},'stream':{'intervalMs':100,'itemsPerMessage':1}},actor='extra',expected=(429,))
 for actor_id,owned in all_sessions:request('DELETE','/streams/'+owned['id'],actor='global-'+actor_id,expected=(204,))
 cleanup();mark('global 32-session admission quota across distinct users')
 for i in range(12):
  info=create();c,r=subscribe(info);assert event(r)['type']=='started';r.close();c.close()
  cleanup()
 mark('abrupt SSE disconnects release both maps')
 # A stopped reader must hit the real SSE socket write deadline (10 seconds).
 large_schema={'$schema':schema['$schema'],'type':'array','minItems':200,'maxItems':200,'items':{'const':'x'*10000}}
 info=create(custom={'schema':large_schema,'generation':{'seed':'backpressure'},'stream':{'intervalMs':100,'itemsPerMessage':1}});c,r=subscribe(info)
 # Do not read body. Waiting for cleanup tests actual TCP backpressure, not a fake writer.
 cleanup(25);r.close();c.close();mark('non-reading SSE client reaches write deadline and cleanup')
 # Only harness-owned processes are restarted. No local application process is stopped.
 for victim in ('mock','backend'):
  info=create();c,r=subscribe(info);assert event(r)['type']=='started';stop(victim);r.close();c.close();start(victim);time.sleep(2)
  request('GET','/streams/'+info['id'],expected=(404,503));cleanup();mark(victim+' restart drops old sessions')
 # Mixed phase lasts at least requested duration, independent of earlier checks.
 # Both processes were restarted: warm HTTP/gRPC connection and worker pools again.
 list(pool.map(cycle,range(100)))
 mixed_start=time.monotonic();mixed=0;initial=cleanup();report['mixedBaseline']=initial;stacks('initial')
 while time.monotonic()-mixed_start<a.duration:
  list(pool.map(cycle,range(mixed,mixed+4)))
  mixed+=4
  if mixed%40==0:sample()
  time.sleep(.1)
 final=cleanup();report['mixedFinal']=final;report['mixedCycles']=mixed;report['mixedSeconds']=round(time.monotonic()-mixed_start,2);stacks('final');mark(str(a.duration)+' seconds mixed load; '+str(mixed)+' cycles')
 for name in ('mock','backend'):
  assert final[name]['goroutines']<=initial[name]['goroutines']+12,(name,'goroutine growth',initial[name],final[name])
  assert final[name]['heap']<=initial[name]['heap']+64*1024*1024,(name,'heap growth')
 report['result']='passed'
except BaseException as e:
 report['result']='failed';report['errors'].append(repr(e));raise
finally:
 report['elapsedSeconds']=round(time.monotonic()-begun,2)
 if report['latenciesMs']:
  ordered=sorted(report.pop('latenciesMs'));report['latencyMs']={'count':len(ordered),'p50':ordered[len(ordered)//2],'p95':ordered[int(len(ordered)*.95)],'max':max(ordered)}
 pool.shutdown(wait=True);close_connections()
 for name in list(procs):stop(name)
 for name in ('mock','backend'):
  logpath=pathlib.Path(str(out)+'.'+name+'.log')
  if logpath.exists() and 'panic:' in logpath.read_text():report['result']='failed';report['errors'].append(name+' panic')
 out.write_text(json.dumps(report,indent=2)+'\n');print('report: '+str(out),flush=True)

if report["result"] != "passed":
 raise SystemExit(1)

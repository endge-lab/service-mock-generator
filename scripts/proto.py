#!/usr/bin/env python3
"""Regenerate/check canonical mockdata.v1 and the backend's generated client."""
import argparse,hashlib,pathlib,subprocess,tempfile
p=argparse.ArgumentParser();p.add_argument('--write',action='store_true');a=p.parse_args()
root=pathlib.Path(__file__).resolve().parents[1];backend=root.parent/'egorkozelskij-endge-service-backend/internal/adapter/mockpb'
source=root/'api/proto/mockdata/v1/mockdata.proto';digest=hashlib.sha256(source.read_bytes()).hexdigest()+'\n'
with tempfile.TemporaryDirectory(prefix='mock-proto-') as folder:
 subprocess.run(['protoc','-I',str(root/'api/proto'),'--go_out='+folder,'--go_opt=paths=source_relative','--go-grpc_out='+folder,'--go-grpc_opt=paths=source_relative',str(source)],check=True)
 for name in ['mockdata.pb.go','mockdata_grpc.pb.go']:
  data=(pathlib.Path(folder)/'mockdata/v1'/name).read_bytes()
  for target in [root/'api/mockdata/v1'/name,backend/name]:
   if a.write:target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(data)
   elif not target.exists() or target.read_bytes()!=data:raise SystemExit('Generated contract drift: '+str(target))
 for target in [root/'api/proto/mockdata.v1.sha256',backend/'mockdata.v1.sha256']:
  if a.write:target.write_text(digest)
  elif not target.exists() or target.read_text()!=digest:raise SystemExit('Contract hash drift: '+str(target))
print('mockdata.v1 canonical source, generated server and backend client match')

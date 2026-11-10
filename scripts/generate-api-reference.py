#!/usr/bin/env python3
"""Generate source-level API lookup tables; never used in the application runtime."""
from pathlib import Path
import re,json
ROOT=Path(__file__).resolve().parents[1]
SRC=ROOT/'crates/music-server/src'

def balanced(s,start,left='(',right=')'):
    depth=0
    for m in re.finditer(r'''r(\#{0,16})"[\s\S]*?"\1|"(?:\\.|[^"\\])*"|b?'(?:\\.|[^'\\])'|//[^\n]*|/\*[\s\S]*?\*/|[(){}]''',s[start:]):
        if m[0]==left:depth+=1
        elif m[0]==right:
            depth-=1
            if depth==0:return s[start:start+m.end()]
    raise ValueError('Unclosed source expression')

source=(SRC/'lib.rs').read_text();routes=[]
for m in re.finditer(r'\.route\(',source):
    expr=balanced(source,m.end()-1)
    path=re.match(r'\(\s*"([^"]+)"',expr)
    if not path:continue
    for method,handler in re.findall(r'\b(get|post|put|patch|delete|head|options)\(\s*([a-zA-Z_][a-zA-Z0-9_:]*)\s*\)',expr):
        routes.append({'method':method.upper(),'path':path[1],'handler':handler})
assert len({(o['method'],o['path']) for o in routes})==len(routes)
functions={};definitions=[]
for file in sorted(SRC.rglob('*.rs')):
    text=file.read_text().split('#[cfg(test)]',1)[0];module='::'.join(file.relative_to(SRC).with_suffix('').parts)
    if module=='lib':module=''
    for m in re.finditer(r'(?:pub(?:\([^)]*\))?\s+)?async\s+fn\s+(\w+)\s*\(',text):
        args=balanced(text,m.end()-1);body_start=text.find('{',m.end()-1+len(args))
        if body_start<0:continue
        name=(module+'::' if module else '')+m[1]
        functions[name]={'signature':text[m.start():body_start].strip(),'source':str(file.relative_to(ROOT))+'#L'+str(text[:m.start()].count('\n')+1),'response_types':sorted(set(re.findall(r'Json\s*<\s*([A-Za-z_]\w*)',text[m.start():body_start])))}
    for m in re.finditer(r'(?P<attrs>(?:#\[[^\n]+\]\s*)+)(?:pub\s+)?(?P<kind>struct|enum)\s+(?P<name>\w+)\s*\{',text):
        if not re.search(r'Serialize|Deserialize',m['attrs']):continue
        decl=m['attrs']+(m['kind']+' '+m['name']+' ')+balanced(text,m.end()-1,'{','}')
        definitions.append({'name':m['name'],'source':str(file.relative_to(ROOT)),'declaration':decl})
for op in routes:op.update(functions.get(op['handler'],{}))
out=ROOT/'app/public/api-catalogue.json';out.parent.mkdir(parents=True,exist_ok=True)
out.write_text(json.dumps({'description':'Native Rust API source reference; VOS gateway restrictions apply. Not OpenAPI.', 'operations':routes,'types':definitions},indent=2,ensure_ascii=False)+'\n')
print(len(routes),'HTTP operations;',len(definitions),'native serialized declarations')

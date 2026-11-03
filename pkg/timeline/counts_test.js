'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const html=fs.readFileSync('web.html','utf8').replace('COUNTS_PLACEHOLDER',fs.readFileSync('counts.js','utf8'));
const {Counts,Timeline}=vm.runInNewContext(html.match(/<script>\n([\s\S]*?)<\/script>/)[1]+';({Counts,Timeline});');
const key=time=>String(time).padStart(4,'0');
const selected={startKey:key(10),endKey:key(20)};
const event=(start,end=start,overrides={})=>({
  source:'KubeEvent',locator:{type:'Kind',keys:{namespace:'ns',deployment:'oauth'}},
  message:{reason:'ScaleReplicaUp',humanMessage:'Scaled up',annotations:{}},
  startKey:key(start),endKey:key(end),...overrides
});
const group=(items,test=selected,matches=()=>true)=>Counts.group(items,test,matches);

test('flat resource/event rows compare all entries with the selected test',()=>{
  const rows=group([event(0),event(10),event(15),event(20),event(30)]);
  assert.equal(rows.length,1);
  const row=rows[0];
  assert.equal(row.source,'KubeEvent');assert.equal(row.namespace,'ns');
  assert.equal(row.resource,'deployment=oauth');assert.equal(row.event,'ScaleReplicaUp');
  assert.equal(row.total,5);assert.equal(row.inTest,3);
});
test('exact inclusive overlap includes endpoints and spanning intervals without display margin',()=>{
  const items=[event(0,10),event(20,30),event(0,30),event(10),event(20),event(9),event(21),
    {...event(10),startKey:'0009.999999999',endKey:'0009.999999999'},
    {...event(20),startKey:'0020.000000001',endKey:'0020.000000001'}];
  assert.equal(group(items)[0].total,9);assert.equal(group(items)[0].inTest,5);
  assert.equal(group([event(9,30,{to:null,openEnd:true})])[0].inTest,1);
});
test('source, namespace, resource kind/name, reason and cause distinguish groups',()=>{
  const items=[event(15),event(15,15,{source:'PodState'}),
    event(15,15,{locator:{keys:{namespace:'other',deployment:'oauth'}}}),
    event(15,15,{locator:{keys:{namespace:'ns',deployment:'other'}}}),
    event(15,15,{locator:{keys:{namespace:'ns',pod:'oauth'}}}),
    event(15,15,{message:{reason:'ScaleReplicaDown'}}),
    event(15,15,{message:{reason:'ScaleReplicaUp',cause:'different cause'}})];
  assert.equal(group(items).length,items.length);
  assert.equal(new Set(group(items).map(row=>row.key)).size,items.length);
});
test('different messages and metadata do not split the same resource/event group',()=>{
  const rows=group([event(15),event(30,30,{
    locator:{keys:{deployment:'oauth',namespace:'ns',hmsg:'different-hash'}},
    message:{reason:'ScaleReplicaUp',humanMessage:'Other replica counts and pod names'}
  })]);
  assert.equal(rows.length,1);assert.equal(rows[0].total,2);assert.equal(rows[0].inTest,1);
});
test('source, namespace and text filters affect both counts',()=>{
  const items=[event(15),event(30),event(15,15,{source:'PodState'}),
    event(15,15,{locator:{keys:{namespace:'other',deployment:'oauth'}}}),
    event(15,15,{message:{reason:'Ready',humanMessage:'Ready'}})];
  const rows=group(items,selected,item=>Timeline.matches(item,'KubeEvent','ns','SCALEREPLICAUP'));
  assert.equal(rows.length,1);assert.equal(rows[0].total,2);assert.equal(rows[0].inTest,1);
  assert.equal(group(items,selected,()=>false).length,0);
});
test('each timeline entry counts once regardless of cumulative counters or duplicate observations',()=>{
  const items=['2','3','9007199254740992',undefined].map(count=>
    event(15,15,{message:{reason:'ScaleReplicaUp',annotations:{uid:'same',count}}}));
  items.push(items[0]);
  assert.equal(group(items)[0].total,5);assert.equal(group(items)[0].inTest,5);
});
test('missing test differs from a selected test without overlapping events',()=>{
  assert.equal(group([event(30)],null)[0].inTest,null);
  assert.equal(group([event(30)])[0].inTest,0);
  assert.equal(group([]).length,0);
});
test('explicit resource identities, test names and absent fields',()=>{
  assert.equal(Counts.identity(event(1,1,{locator:{keys:{pod:'oauth-123',container:'server'}}})).resource,'container=server · pod=oauth-123');
  assert.equal(Counts.identity(event(1,1,{locator:{type:'Widget',keys:{name:'explicit'}}})).resource,'Widget=explicit');
  const missing=Counts.identity(event(1,1,{locator:{},message:{humanMessage:'Original message'}}));
  assert.equal(missing.resource,'(no resource)');assert.equal(missing.namespace,'(no namespace)');assert.equal(missing.event,'Original message');
  const tests=['test A','test B'].map(name=>event(15,15,{source:'E2ETest',locator:{keys:{'e2e-test':name}},message:{}}));
  assert.equal(group(tests).length,2);
});

test('invalid intervals count in totals but never in temporal overlap',()=>{
  const invalid=event(15,16,{valid:false,reason:'ends before it starts'});
  const row=group([event(15),invalid])[0];
  assert.equal(row.total,2);assert.equal(row.inTest,1);
  assert.equal(Timeline.overlaps(invalid,selected),false);
  assert.equal(Timeline.overlaps(event(15),{...selected,valid:false}),false);
  const viewport=Timeline.viewport({start:10,end:20},[{start:-1e12,end:1e12,valid:false}],true);
  assert.ok(viewport[0]>0&&viewport[1]<30);
});

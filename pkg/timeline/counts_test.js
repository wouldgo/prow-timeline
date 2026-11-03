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

const snapshot=(count,hmsg='hash',uid='event-uid',time=15)=>event(time,time,{
  locator:{keys:{namespace:'ns',deployment:'oauth',hmsg}},
  message:{reason:'ScaleReplicaUp',humanMessage:'Scaled '+hmsg,
    annotations:{uid,count,firstTimestamp:'first',lastTimestamp:String(time),pathological:'true'}}
});
test('cumulative counters take the maximum per UID across changed messages and duplicated artifacts',()=>{
  const items=[snapshot('79','a'),snapshot('97','b'),snapshot('103','c')];
  const row=group([...items,...items])[0];
  assert.equal(row.total,6);
  assert.equal(row.cumulative,'103');assert.equal(row.counterUIDs,1);
  assert.equal(row.observations.length,3);
  assert.ok(row.observations.every(observation=>observation.records===2));
  assert.equal(row.observations[2].hmsg,'c');
  assert.equal(row.observations[2].pathological,'true');
  assert.equal(group([...items,snapshot('5','d','another')])[0].cumulative,'108');
});
test('counters use matching records, retain exact large integers and never imply test-local occurrences',()=>{
  const items=[snapshot('79','a','same',5),snapshot('97','b','same',15),snapshot('103','c','same',30)];
  const row=group(items)[0];
  assert.equal(row.cumulative,'103');assert.equal(row.inTest,1);
  assert.equal(group(items,selected,e=>e.locator.keys.hmsg==='b')[0].cumulative,'97');
  assert.equal(group([snapshot('9007199254740993'),snapshot('2','b','other')])[0].cumulative,'9007199254740995');
  assert.equal(group([snapshot('103'),snapshot('79')])[0].cumulative,'103');
});
test('missing UID or invalid counts are reported as incomplete, and other sources have no Kubernetes counters',()=>{
  const bad=[snapshot('abc'),snapshot('-1'),snapshot('1.2'),snapshot(''),snapshot('3','hash','')];
  const row=group([...bad,snapshot('103')])[0];
  assert.equal(row.cumulative,'103');assert.equal(row.missingCounters,5);
  assert.equal(group(bad)[0].cumulative,null);
  assert.equal(group([snapshot(undefined)])[0].cumulative,'1');
  assert.equal(group([snapshot(undefined)])[0].observations[0].countDefaulted,true);
  const other=group([{...snapshot('103'),source:'PodState'}])[0];
  assert.equal(other.cumulative,null);assert.equal(other.missingCounters,0);
});
test('missing test differs from a selected test without overlapping events',()=>{
  assert.equal(group([event(30)],null)[0].inTest,null);
  assert.equal(group([event(30)])[0].inTest,0);
  assert.equal(group([]).length,0);
});

test('selected test shows cumulative history up to its end, a prior baseline and per-UID increments',()=>{
  const items=[snapshot('79','a','same',5),snapshot('97','b','same',12),snapshot('103','c','same',18),snapshot('150','d','same',30)];
  const row=group([...items,...items])[0];
  assert.equal(row.cumulative,'150');assert.equal(row.testProgress.soFar,'103');
  const steps=row.testProgress.observations;
  assert.deepEqual(Array.from(steps,s=>s.count),['79','97','103']);
  assert.deepEqual(Array.from(steps,s=>s.position),['Before test','In test','In test']);
  assert.deepEqual(Array.from(steps,s=>s.increase),[null,'18','6']);
  assert.equal(steps[1].boundedByTest,false);assert.equal(steps[2].boundedByTest,true);
  assert.equal(steps[2].records,2);
  assert.equal(group(items,null)[0].testProgress,null);
});
test('test progression uses exact occurrence points, excludes invalid times and preserves boundaries',()=>{
  const items=[snapshot('2','a','same',0),snapshot('4','b','same',5),snapshot('5','c','same',10),
    snapshot('8','d','same',20),snapshot('9','e','same',21),
    {...snapshot('999','bad','same',15),valid:false},snapshot('2','other','other',15)];
  // A one-second drawing interval can overlap a test even when its occurrence is before it.
  items[1].endKey=key(15);
  const progress=group(items)[0].testProgress;
  assert.equal(progress.soFar,'10');
  assert.equal(progress.observations.filter(s=>s.uid==='same'&&s.position==='Before test').length,1);
  assert.deepEqual(Array.from(progress.observations.filter(s=>s.uid==='same'),s=>s.count),['4','5','8']);
  assert.equal(progress.observations.find(s=>s.uid==='other').increase,null);
  assert.equal(group([snapshot('1','future','same',30)])[0].testProgress.soFar,null);
});
test('counter decreases do not become negative occurrences or an invented increment',()=>{
  const progress=group([snapshot('97','a','same',5),snapshot('3','b','same',12),snapshot('9','c','same',18)])[0].testProgress;
  assert.equal(progress.observations[1].increase,null);
  assert.equal(progress.observations[1].counterDecreased,true);
  assert.equal(progress.observations[2].increase,'6');
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

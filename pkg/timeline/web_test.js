'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const html=fs.readFileSync('web.html','utf8').replace('COUNTS_PLACEHOLDER',fs.readFileSync('counts.js','utf8'));
const script=html.match(/<script>\n([\s\S]*?)<\/script>/)[1];
const model=vm.runInNewContext(script+';Timeline;');
test('inclusive overlap: endpoints, points, enclosing intervals, outside',()=>{
  const selected={start:10,end:20};
  for(const [start,end,want] of [[0,10,true],[20,30,true],[10,10,true],[20,20,true],[0,30,true],[12,15,true],[0,9,false],[21,25,false]]){
    assert.equal(model.overlaps({start,end},selected),want,`${start}–${end}`);
  }
});
test('clipping indicates both continuations and preserves originals',()=>{
  const event={start:0,end:30};const clip=model.clip(event,10,20);
  assert.equal(clip.left,0);assert.equal(clip.right,100);assert.equal(clip.before,true);assert.equal(clip.after,true);
  assert.deepEqual(event,{start:0,end:30});
});
test('open ends retain continuation and label observed horizon explicitly',()=>{
  const event={start:10,end:20,to:null,startKey:'2026-11-02T17:11:09.000000000Z',endKey:'2026-11-02T17:11:13.000000000Z',openEnd:true};
  assert.equal(model.clip(event,0,30).after,true);
  assert.match(model.endLabel(event),/open; observed horizon/);
  assert.equal(event.to,null);
  assert.equal(model.overlaps(event,{startKey:event.endKey,endKey:event.endKey}),true);
  assert.equal(model.overlaps(event,{startKey:'2026-11-02T17:11:13.000000001Z',endKey:'2026-11-02T17:11:13.000000001Z'}),false);
});
test('focus and entire-run windows including point tests',()=>{
  const selected={start:10,end:20},outside={start:0,end:100};
  const focus=model.viewport(selected,[outside],false),all=model.viewport(selected,[outside],true);
  assert.ok(focus[0]<10&&focus[1]>20);assert.ok(all[0]<0&&all[1]>100);
  const point=model.viewport({start:10,end:10},[],false);assert.ok(point[0]<10&&point[1]>10);
});
test('filters category, namespace and case-insensitive resource/message text',()=>{
  const event={source:'PodState',locator:{keys:{namespace:'ns',pod:'my-pod'}},message:{humanMessage:'Ready'}};
  assert.equal(model.matches(event,'PodState','ns','READY'),true);
  assert.equal(model.matches(event,'KubeEvent','',''),false);
  assert.equal(model.matches(event,'','other',''),false);
  assert.equal(model.matches(event,'','','missing'),false);
  assert.ok(model.resource(event).includes('my-pod'));
});
test('test classification ignores status on non-test sources',()=>{
  assert.equal(model.isTest({source:'PodState',locator:{keys:{'e2e-test':'x'}}}),false);
  assert.equal(model.isTest({source:'E2ETest',locator:{keys:{'e2e-test':'x'}}}),true);
  assert.equal(model.isTest({source:'E2ETest',locator:{}}),false);
});
test('timeline collapses only identical records, independent of JSON object key order',()=>{
  const first={source:'KubeEvent',from:'time',to:null,display:false,
    locator:{keys:{namespace:'ns',deployment:'oauth',hmsg:'a'}},
    message:{humanMessage:'Scaled up',annotations:{uid:'same',count:'97'}}};
  const reordered={message:{annotations:{count:'97',uid:'same'},humanMessage:'Scaled up'},
    locator:{keys:{hmsg:'a',deployment:'oauth',namespace:'ns'}},display:false,to:null,from:'time',source:'KubeEvent'};
  const distinct=[{...first,from:'other time'},{...first,to:'time'},
    {...first,display:true},{...first,source:'Other'},
    {...first,locator:{keys:{...first.locator.keys,hmsg:'b'}}},
    {...first,message:{...first.message,annotations:{uid:'same',count:'103'}}},
    {...first,message:{...first.message,annotations:{uid:'other',count:'97'}}},
    {...first,message:{...first.message,humanMessage:'Scaled down'}}];
  const original=JSON.stringify(first);
  const lanes=model.coalesce([first,reordered,...distinct]);
  assert.equal(lanes.length,1+distinct.length);
  assert.equal(lanes[0].event,first);assert.equal(lanes[0].copies,2);
  assert.ok(lanes.slice(1).every(lane=>lane.copies===1));
  assert.equal(JSON.stringify(first),original);
});
test('page has bounded event DOM and no external assets or unsafe sinks',()=>{
  assert.match(html,/const pageSize=60/);assert.match(html,/visible.slice\(page\*pageSize,/);
  assert.doesNotMatch(html,/innerHTML|document.write|<script[^>]+src=|<link[^>]+href=|fetch\(/);
  assert.match(html,/<details id="eventDetails" class="panel" aria-label="Event details"><summary><h2>Event details<\/h2><\/summary>/);
});

test('nanosecond comparisons exclude points just outside either endpoint',()=>{
  const selected={start:0,end:0,startKey:'2026-11-02T17:11:09.000000001Z',endKey:'2026-11-02T17:11:13.000000001Z'};
  for(const [stamp,want] of [['2026-11-02T17:11:09.000000000Z',false],['2026-11-02T17:11:09.000000001Z',true],['2026-11-02T17:11:13.000000001Z',true],['2026-11-02T17:11:13.000000002Z',false]]){
    assert.equal(model.overlaps({start:0,end:0,startKey:stamp,endKey:stamp},selected),want,stamp);
  }
});

test('native UI runtime: pagination, test selection/details, search, filters and run changes',()=>{
  class Element {
    constructor(tag){this.tag=tag;this.children=[];this.style={};this.value='';this.className='';this.listeners={};this._text='';this.open=false;}
    get options(){return this.children;}
    get selectedOptions(){return this.children.filter(option=>option.selected);}
    get value(){return this.tag==='select'?String(this.selectedOptions[0]?.value??''):this._value;}
    set value(value){this._value=String(value);if(this.tag==='select')for(const option of this.children)option.selected=String(option.value)===String(value);}
    set textContent(value){this._text=String(value);this.children=[];}
    get textContent(){return this._text+this.children.map(child=>child.textContent).join('');}
    append(...children){for(const child of children){if(this.tag==='select'&&this.children.length===0)child.selected=true;this.children.push(child);}}
    replaceChildren(...children){this.children=[];this._text='';this.value='';this.append(...children);}
    setAttribute(name,value){this[name]=value;}
    addEventListener(name,handler){this.listeners[name]=handler;}
  }
  const elements=new Map();
  const selects=new Set(['run','tests','category','namespace']);
  const get=id=>{assert.ok(html.includes('id="'+id+'"'),'Unknown element '+id);if(!elements.has(id))elements.set(id,new Element(selects.has(id)?'select':'div'));return elements.get(id);};
  const key=time=>new Date(time).toISOString().replace(/(\.\d{3})Z/,'$1000000Z');
  const event=(source,name,start,end)=>({source,level:'Info',locator:{type:source,keys:source==='E2ETest'?{'e2e-test':name}:{namespace:'ns',pod:name}},message:{reason:'Ready',humanMessage:'<script>untrusted</script>',annotations:{status:'Passed'}},start,end,from:new Date(start).toISOString(),to:new Date(end).toISOString(),startKey:key(start),endKey:key(end)});
  const items=[event('E2ETest','Alpha',1000,2000),event('E2ETest','Beta',3000,4000),event('E2ETest','Alpha',5000,6000),...Array.from({length:125},(_,index)=>event('PodState','pod-'+index,900,2100)),event('KubeEvent','outside',0,0)];
  items.push(...Array.from({length:305},(_,index)=>event('E2ETest','Extra-'+index,10000+index*100,10001+index*100)));
  items.push({...event('PodState','open-pod',900,40401),to:null,openEnd:true});
  items.push({...event('E2ETest','Open test',7000,40401),to:null,openEnd:true});
  const testCount=items.filter(item=>item.source==='E2ETest').length;
  const data={jobURL:'https://example.invalid/untrusted',eventCount:items.length+1,testCount,runs:[{name:'step/run1',testCount,items},{name:'step/run2',testCount:0,items:[event('PodState','other',1000,2000)]}]};
  data.runs[1].items.push(...Array.from({length:61},(_,index)=>({
    ...event(index===0?'E2ETest':'MachineMonitor','invalid-'+index,3000,1000),
    valid:false,reason:'ends before it starts',start:0,end:0,startKey:'',endKey:''
  })));
  const otherItems=[event('E2ETest','Repeated',1000,2000),event('E2ETest','Repeated',1000,2000),...['2','3','4'].map((count,index)=>{
    const record=event('KubeEvent','shared',1100+index,1900);record.message.annotations={uid:'shared',count};return record;
  })];
  otherItems.push(...Array.from({length:65},(_,index)=>{
    const record=event('KubeEvent','uid-'+index,1200,1200);record.message.annotations={uid:'uid-'+index,count:'1'};return record;
  }));
  const duplicate=event('KubeEvent','duplicated',1100,1200);
  duplicate.message.annotations={uid:'duplicated',count:'97'};
  const later=event('KubeEvent','duplicated',1300,1400);later.message.annotations={uid:'duplicated',count:'103'};
  otherItems.push(duplicate,JSON.parse(JSON.stringify(duplicate)),later);
  data.runs.push({name:'step/run3',testCount:2,items:otherItems});data.eventCount+=otherItems.length;data.testCount+=2;
  const multiItems=[event('E2ETest','Early',1000,2000),event('E2ETest','Late',3000,4000),
    event('PodState','early-a',1100,1200),event('PodState','late-b',3100,3200),
    event('PodState','gap',2500,2500),event('PodState','spanning',1500,3500),
    {...event('PodState','invalid',3000,1000),valid:false,reason:'invalid',startKey:'',endKey:''}];
  for(const item of multiItems.filter(item=>item.source==='PodState'))item.locator.keys.namespace=
    item.locator.keys.pod==='late-b'||item.locator.keys.pod==='gap'?'b':item.valid===false?'c':'a';
  data.runs.push({name:'step/multi',testCount:2,items:multiItems});
  get('data').textContent=JSON.stringify(data);get('window').value='focus';
  const context=vm.createContext({document:{getElementById:get,createElement:tag=>new Element(tag)}});
  const scripts=[...html.matchAll(/<script>\n([\s\S]*?)<\/script>/g)].map(match=>match[1]);
  vm.runInContext(scripts.join('\n'),context);
  const rows=()=>get('rows').children.filter(child=>child.className==='event-row');
  assert.equal(get('eventDetails').open,false);
  assert.equal(rows().length,60);assert.match(get('page').textContent,/1 \/ 3/);
  const namespaceLabel=rows()[0].children[0].children.at(-1);
  assert.equal(namespaceLabel.className,'event-namespace');assert.equal(namespaceLabel.textContent,'Namespace: ns');
  assert.match(rows()[0]['aria-label'],/Namespace: ns/);assert.match(rows()[0].title,/Namespace: ns/);
  rows()[0].onclick();assert.equal(get('eventDetails').open,true);
  const inspected=get('details').textContent,selection=vm.runInContext('JSON.stringify(selectedValues("tests"))',context);
  get('eventDetails').open=false;
  vm.runInContext('draw(false)',context);
  assert.equal(get('eventDetails').open,false);assert.equal(get('details').textContent,inspected);
  assert.equal(vm.runInContext('JSON.stringify(selectedValues("tests"))',context),selection);
  rows()[1].onclick();assert.equal(get('eventDetails').open,true);
  const displayedStarts=[];
  do {
    for(const row of rows()){
      row.onclick();
      displayedStarts.push(JSON.parse(get('details').textContent).from);
    }
    if(get('next').disabled)break;
    get('next').onclick();
  } while(true);
  assert.deepEqual(displayedStarts,[...displayedStarts].sort(),'timeline rows follow start time across pages');
  assert.equal(vm.runInContext('JSON.stringify(data)',context),JSON.stringify(data),'display ordering preserves original data');
  while(!get('prev').disabled)get('prev').onclick();
  assert.equal(get('countRows').children.length,60);
  assert.match(get('countsPage').textContent,/1 \/ /);
  assert.match(get('countSummary').textContent,/resource\/event groups/);
  const firstCountsPage=get('countRows').textContent;
  get('next').onclick();assert.match(get('page').textContent,/2 \/ 3/);assert.equal(rows().length,60);
  assert.equal(get('countRows').textContent,firstCountsPage);
  get('countsNext').onclick();assert.match(get('countsPage').textContent,/2 \/ /);
  assert.match(get('page').textContent,/2 \/ 3/);
  get('countsPrev').onclick();assert.equal(get('countRows').textContent,firstCountsPage);
  rows()[0].onclick();assert.equal(JSON.parse(get('details').textContent).intersectsSelectedTest,true);
  assert.match(get('details').textContent,/<script>untrusted<\/script>/);
  get('tests').value='1';get('tests').onchange();assert.match(get('testInfo').textContent,/Beta/);assert.equal(get('details').textContent,'Select an event lane to inspect details.');
  get('testSearch').value='no such test';get('testSearch').oninput();assert.equal(rows().length,0);assert.match(get('summary').textContent,/No test selected/);
  get('testSearch').value='Alpha';get('testSearch').oninput();assert.equal(get('tests').children.length,2);assert.match(get('tests').children[1].textContent,/attempt 2/);
  get('testSearch').value='';get('testSearch').oninput();
  const openRow=rows().find(row=>row.textContent.includes('open-pod'));assert.ok(openRow);openRow.onclick();
  const openDetails=JSON.parse(get('details').textContent);assert.equal(openDetails.to,null);assert.equal(openDetails.openEnd,true);assert.equal(openDetails.effectiveEndUTC,key(40401));
  get('category').value='KubeEvent';get('category').listeners.change();assert.equal(rows().length,0);
  get('window').value='all';get('window').listeners.change();assert.equal(rows().length,1);
  get('run').value='1';get('run').onchange();assert.equal(rows().length,0);assert.match(get('summary').textContent,/No test selected/);
  get('run').value='0';get('run').onchange();assert.equal(rows().length,60);assert.equal(get('category').value,'');
  get('testSearch').value='Extra-304';get('testSearch').oninput();assert.match(get('testInfo').textContent,/Extra-304/);
  get('testSearch').value='';get('testSearch').oninput();assert.equal(get('tests').children.length,301);
  assert.ok(get('tests').children.some(child=>String(child.value)===get('tests').value));
  assert.match(get('testInfo').textContent,/Extra-304/);
  get('testSearch').value='Open test';get('testSearch').oninput();assert.match(get('testInfo').textContent,/observed span/);
  get('text').value='Open test';get('text').listeners.input();assert.equal(rows().length,1);rows()[0].onclick();
  const selectedOpenDetails=JSON.parse(get('details').textContent);assert.equal(selectedOpenDetails.to,null);assert.equal(selectedOpenDetails.openEnd,true);assert.equal(selectedOpenDetails.effectiveEndUTC,key(40401));
  assert.match(get('rows').children.at(-1).children.at(-1).title,/observed horizon/);
  vm.runInContext('showDetails(data.runs[2].items[2])',context);
  const crossArtifact=JSON.parse(get('details').textContent);
  assert.equal(crossArtifact.artifact,'step/run3');assert.deepEqual(crossArtifact.activeTests.map(test=>test.attempt),[1,2]);
  assert.ok(crossArtifact.activeTests.every(test=>test.artifact==='step/run3'));
  get('text').value='';get('category').value='KubeEvent';get('text').listeners.input();
  const findGroup=name=>get('countRows').children.find(row=>row.children[2].textContent==='pod='+name);
  const shared=findGroup('shared');assert.ok(shared);
  assert.equal(shared.children[4].textContent,'3');
  assert.equal(shared.children[5].textContent,'0');
  assert.equal(shared.children[6].textContent,'4');
  get('eventDetails').open=false;shared.children[8].children[0].onclick();
  assert.equal(get('eventDetails').open,true);
  const counterDetails=JSON.parse(get('details').textContent);
  assert.equal(counterDetails.cumulative,'4');assert.equal(counterDetails.counterUIDs,1);
  assert.deepEqual(counterDetails.observations.map(item=>item.count),['2','3','4']);
  assert.equal(counterDetails.recordsInSelectedTest,0);
  assert.match(counterDetails.note,/not the selected test/);
  assert.ok(counterDetails.observations.every(item=>item.message.includes('<script>untrusted</script>')));
  assert.equal(shared.children[7].textContent,'4View progression');
  shared.children[7].children[0].onclick();
  assert.equal(get('counterProgress').hidden,false);
  assert.equal(get('progressRows').children.length,1);
  assert.equal(get('progressRows').children[0].children[3].textContent,'4');
  assert.equal(get('progressRows').children[0].children[2].textContent,'Before test');
  get('clearTest').onclick();
  assert.equal(get('counterProgress').hidden,true);
  assert.match(get('countSummary').textContent,/Select a test/);
  assert.ok(get('countRows').children.every(row=>row.children[5].textContent==='—'));
  get('testSearch').value='Alpha';get('testSearch').oninput();
  assert.equal(findGroup('shared').children[5].textContent,'3');
  assert.equal(get('counterProgress').hidden,false);
  assert.equal(get('progressRows').children.length,3);
  assert.equal(get('progressRows').children[1].children[4].textContent,'+1');
  assert.equal(get('progressRows').children[1].children[5].textContent,'Within test');
  const absent= new Element('option');absent.value='not-present';get('namespace').append(absent);get('namespace').value='not-present';get('namespace').listeners.change();
  assert.equal(get('countRows').children.length,0);
  assert.equal(get('countsPage').textContent,'0 / 0');
  assert.equal(get('countsPrev').disabled,true);assert.equal(get('countsNext').disabled,true);
  assert.equal(get('countSummary').textContent,'No events match the filters.');
  get('namespace').value='';get('category').value='';get('text').value='untrusted';get('text').listeners.input();
  assert.ok(get('countRows').children.length>0);
  assert.doesNotMatch(html,/Captured artifact data|occurrence ledger|groupRecords|ledgerPrev/);
  get('run').value='1';get('run').onchange();
  assert.equal(rows().length,0);assert.match(get('summary').textContent,/No test selected/);
  assert.equal(get('tests').children.length,0);
  assert.match(get('invalidSummary').textContent,/61 invalid intervals/);
  assert.equal(get('invalidRows').children.length,60);
  assert.equal(get('invalidPage').textContent,'1 / 2');
  const invalidRow=get('invalidRows').children[0];
  assert.match(invalidRow.textContent,/Invalid.*ends before it starts/);
  invalidRow.onclick();
  const invalidDetails=JSON.parse(get('details').textContent);
  assert.equal(invalidDetails.valid,false);assert.equal(invalidDetails.reason,'ends before it starts');
  assert.equal(invalidDetails.from,new Date(3000).toISOString());
  assert.equal(invalidDetails.to,new Date(1000).toISOString());
  assert.deepEqual(invalidDetails.activeTests,[]);assert.equal(invalidDetails.relativeStartSeconds,null);
  get('invalidNext').onclick();assert.equal(get('invalidRows').children.length,1);
  assert.equal(get('invalidPage').textContent,'2 / 2');assert.equal(get('invalidNext').disabled,true);
  get('invalidPrev').onclick();assert.equal(get('invalidRows').children.length,60);
  get('text').value='invalid-60';get('text').listeners.input();
  assert.equal(get('invalidRows').children.length,1);
  get('invalidRows').children[0].onclick();
  assert.match(get('details').textContent,/invalid-60/);
  get('text').value='no match';get('text').listeners.input();
  assert.equal(get('invalidRows').children.length,0);assert.equal(get('invalidPage').textContent,'0 / 0');

  get('text').value='';get('testSearch').value='';get('run').value='2';get('run').onchange();
  get('text').value='duplicated';get('text').listeners.input();
  assert.equal(rows().length,2);
  const duplicateRow=rows().find(row=>row.textContent.includes('2 copies'));assert.ok(duplicateRow);
  assert.match(duplicateRow['aria-label'],/2 copies/);
  assert.equal(duplicateRow.children[0].children[1].className,'event-reason');
  assert.equal(duplicateRow.children[0].children[1].textContent,'Ready');
  assert.match(duplicateRow['aria-label'],/Ready/);
  duplicateRow.onclick();
  const duplicateDetails=JSON.parse(get('details').textContent);
  assert.equal(duplicateDetails.identicalRecords,2);assert.equal(duplicateDetails.message.annotations.count,'97');
  rows().find(row=>row!==duplicateRow).onclick();
  assert.equal(JSON.parse(get('details').textContent).message.annotations.count,'103');
  assert.equal(JSON.parse(get('details').textContent).identicalRecords,1);
  assert.match(get('summary').textContent,/2 matching intervals.*3 records/);
  assert.equal(findGroup('duplicated').children[4].textContent,'3');
  get('run').value='0';get('run').onchange();get('text').value='duplicated';get('text').listeners.input();
  assert.equal(rows().length,0);

  get('text').value='';get('run').value='3';get('run').onchange();get('window').value='focus';
  for(const option of get('tests').options)option.selected=true;
  get('tests').onchange();
  assert.match(get('testInfo').textContent,/Early[\s\S]*Late/);
  assert.ok(rows().filter(row=>row.textContent.includes('E2ETest')).every(row=>row.children[0].children.at(-1).textContent==='Namespace: —'));
  assert.equal(get('rows').children.at(-1).children.length,6,'each selected test has its own band and boundaries');
  const chooseNamespaces=values=>{
    for(const option of get('namespace').options)option.selected=values.includes(String(option.value));
    get('namespace').listeners.change();
  };
  chooseNamespaces(['a','b']);
  assert.equal(rows().length,3,'union includes both test windows and excludes the gap');
  assert.ok(rows().some(row=>row.textContent.includes('early-a')));
  assert.ok(rows().some(row=>row.textContent.includes('late-b')));
  assert.ok(!rows().some(row=>row.textContent.includes('gap')));
  rows().find(row=>row.textContent.includes('spanning')).onclick();
  assert.deepEqual(JSON.parse(get('details').textContent).selectedTestOverlaps.map(test=>test.intersects),[true,true]);
  const spanning=findGroup('spanning');assert.equal(spanning.children[5].textContent,'1','overlap with two tests counts once');
  assert.equal(findGroup('gap').children[5].textContent,'0');
  chooseNamespaces(['a']);assert.equal(rows().length,2);assert.ok(!findGroup('late-b'));
  chooseNamespaces(['b']);assert.equal(rows().length,1);
  chooseNamespaces(['a','c']);assert.equal(get('invalidRows').children.length,1);
  chooseNamespaces(['a','b']);assert.equal(get('invalidRows').children.length,0);
  chooseNamespaces([]);assert.equal(rows().length,5,'empty namespace selection includes all namespaces');
  chooseNamespaces(['','a']);assert.equal(rows().length,5,'All overrides individual namespaces');
  get('tests').value='1';get('tests').onchange();assert.match(get('testInfo').textContent,/Late/);
  assert.doesNotMatch(get('testInfo').textContent,/Early/);
  for(const option of get('tests').options)option.selected=false;
  get('tests').onchange();assert.equal(rows().length,0);
});

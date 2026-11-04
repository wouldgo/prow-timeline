// Counts groups events by resource and compares totals with the selected test.
const Counts = {
  resourceKeys: new Set([
    'container', 'deployment', 'pod', 'node', 'service', 'daemonset',
    'statefulset', 'replicaset', 'replicationcontroller', 'job', 'cronjob',
    'route', 'clusteroperator', 'machine', 'persistentvolume',
    'persistentvolumeclaim', 'poddisruptionbudget', 'horizontalpodautoscaler',
    'etcd-member', 'e2e-test'
  ]),

  // identity uses explicit resource keys and the event type.
  identity(event) {
    const keys = event.locator.keys || {};
    const resources = Object.entries(keys).filter(([key]) => Counts.resourceKeys.has(key));
    if (keys.name) resources.push([keys.kind || event.locator.type || 'resource', keys.name]);
    resources.sort(([a, av], [b, bv]) => a.localeCompare(b) || av.localeCompare(bv));

    const source = event.source || '(no source)';
    const namespace = keys.namespace || '(no namespace)';
    const reason = event.message.reason || event.message.humanMessage || '(no event type)';
    const cause = event.message.cause || '';
    return {
      key: JSON.stringify([source, namespace, resources, reason, cause]),
      source,
      namespace,
      resource: resources.map(([kind, name]) => kind + '=' + name).join(' · ') || '(no resource)',
      event: reason + (cause ? ' · ' + cause : '')
    };
  },

  // Kubernetes counts belong to Event UIDs, even when aggregation changes hmsg/message.
  // Keep the observations inspectable; never sum snapshots of the same counter.
  counters(events, selected) {
    const maxima = new Map(), observations = new Map();
    let missing = 0;
    for (const event of events) {
      if (event.source !== 'KubeEvent') continue;
      const annotations = event.message.annotations || {};
      // The monitor omits count when it is one (GetTimesAnEventHappened also defaults to one).
      const count = annotations.count ?? '1';
      const observation = {
        uid: annotations.uid || '', hmsg: event.locator.keys?.hmsg || '', count,
        countDefaulted: annotations.count === undefined,
        firstTimestamp: annotations.firstTimestamp || '', lastTimestamp: annotations.lastTimestamp || '',
        from: event.from || '', to: event.to ?? null,
        observedAtKey: event.valid === false ? '' : event.startKey || '',
        pathological: annotations.pathological || '', message: event.message.humanMessage || ''
      };
      const key = JSON.stringify(observation);
      if (!observations.has(key)) observations.set(key, {...observation, records: 0});
      observations.get(key).records++;
      if (!observation.uid || typeof count !== 'string' || !/^\d+$/.test(count)) {
        missing++;
        continue;
      }
      const value = BigInt(count);
      if (!maxima.has(observation.uid) || value > maxima.get(observation.uid)) maxima.set(observation.uid, value);
    }
    const snapshots = [...observations.values()].sort((a, b) =>
      (a.observedAtKey || a.from).localeCompare(b.observedAtKey || b.from) || a.uid.localeCompare(b.uid) ||
      (String(a.count).length - String(b.count).length) || String(a.count).localeCompare(String(b.count)) || a.hmsg.localeCompare(b.hmsg));
    return {
      cumulative: maxima.size ? [...maxima.values()].reduce((sum, count) => sum + count, 0n).toString() : null,
      counterUIDs: maxima.size, missingCounters: missing,
      observations: snapshots,
      testProgress: Counts.progression(snapshots, selected)
    };
  },

  // Compare occurrence timestamps, not the one-second interval added for chart display.
  // A delta straddling the test boundary cannot be wholly attributed to this test.
  progression(observations, selected) {
    if (!selected || selected.valid === false || !selected.startKey || !selected.endKey) return null;
    const baselines = new Map(), previous = new Map(), maxima = new Map(), steps = [];
    for (const observation of observations) {
      const {uid, count, observedAtKey} = observation;
      if (!uid || !observedAtKey || typeof count !== 'string' || !/^\d+$/.test(count) || observedAtKey > selected.endKey) continue;
      const value = BigInt(count);
      if (!maxima.has(uid) || value > maxima.get(uid)) maxima.set(uid, value);
      if (observedAtKey < selected.startKey) {
        baselines.set(uid, {...observation, position: 'Before test', increase: null, boundedByTest: false});
      } else {
        const prior = previous.get(uid);
        const decreased = Boolean(prior && value < BigInt(prior.count));
        steps.push({...observation, position: 'In test',
          increase: prior && !decreased ? (value - BigInt(prior.count)).toString() : null,
          boundedByTest: Boolean(prior && prior.observedAtKey >= selected.startKey && !decreased),
          counterDecreased: decreased});
      }
      previous.set(uid, observation);
    }
    return {
      soFar: maxima.size ? [...maxima.values()].reduce((sum, count) => sum + count, 0n).toString() : null,
      observations: [...baselines.values(), ...steps]
    };
  },

  // Each matching entry counts once. The cumulative annotations.count does not change this count.
  // Test counts use the exact bounds of the selected test.
  group(events, selected, matches) {
    const tests = Array.isArray(selected) ? selected : selected ? [selected] : [];
    const groups = new Map();
    for (const event of events) {
      if (!matches(event)) continue;
      const identity = Counts.identity(event);
      if (!groups.has(identity.key)) {
        groups.set(identity.key, {...identity, total: 0, inTest: tests.length ? 0 : null, records: []});
      }
      const group = groups.get(identity.key);
      group.total++;
      group.records.push(event);
      if (tests.some(test => Timeline.overlaps(event, test))) group.inTest++;
    }
    return [...groups.values()].map(({records, ...group}) => ({...group, ...Counts.counters(records, tests.length === 1 ? tests[0] : null)})).sort((a, b) =>
      a.source.localeCompare(b.source) ||
      a.namespace.localeCompare(b.namespace) ||
      a.resource.localeCompare(b.resource) ||
      a.event.localeCompare(b.event)
    );
  }
};

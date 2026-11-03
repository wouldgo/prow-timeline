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

  // Each matching entry counts once. The cumulative annotations.count does not change this count.
  // Test counts use the exact bounds of the selected test.
  group(events, selected, matches) {
    const groups = new Map();
    for (const event of events) {
      if (!matches(event)) continue;
      const identity = Counts.identity(event);
      if (!groups.has(identity.key)) {
        groups.set(identity.key, {...identity, total: 0, inTest: selected ? 0 : null});
      }
      const group = groups.get(identity.key);
      group.total++;
      if (selected && Timeline.overlaps(event, selected)) group.inTest++;
    }
    return [...groups.values()].sort((a, b) =>
      a.source.localeCompare(b.source) ||
      a.namespace.localeCompare(b.namespace) ||
      a.resource.localeCompare(b.resource) ||
      a.event.localeCompare(b.event)
    );
  }
};

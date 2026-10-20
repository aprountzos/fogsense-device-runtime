// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docsSidebar: [
    'intro',
    'getting-started',
    {
      type: 'category',
      label: 'Device Spec',
      collapsed: false,
      items: [
        'device-spec/overview',
        'device-spec/telemetry',
        'device-spec/state',
        'device-spec/commands',
        'device-spec/examples',
      ],
    },
    {
      type: 'category',
      label: 'Runtime Behavior',
      items: [
        'behavior/simulation-loop',
        'behavior/offline-mode',
        'behavior/gateway-communication',
      ],
    },
    {
      type: 'category',
      label: 'API Reference',
      items: [
        'api/device-api',
        'api/gateway-api',
      ],
    },
    {
      type: 'category',
      label: 'Guides',
      items: [
        'guides/docker',
        'guides/mock-gateway',
        'guides/testing',
        'guides/configuration',
      ],
    },
  ],
};

export default sidebars;

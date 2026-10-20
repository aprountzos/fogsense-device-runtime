// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docsSidebar: [
    {
      type: 'category',
      label: 'Start Here',
      collapsed: false,
      items: [
        'intro',
        'getting-started',
      ],
    },
    {
      type: 'category',
      label: 'Architecture & Behavior',
      collapsed: false,
      items: [
        'behavior/simulation-loop',
        'behavior/offline-mode',
        'behavior/gateway-communication',
      ],
    },
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
      label: 'API & Contracts',
      collapsed: false,
      items: [
        'api/contracts',
        'api/device-api',
        'api/gateway-api',
      ],
    },
    {
      type: 'category',
      label: 'Guides',
      collapsed: false,
      items: [
        'guides/configuration',
        'guides/docker',
        'guides/mock-gateway',
        'guides/testing',
      ],
    },
  ],
};

export default sidebars;

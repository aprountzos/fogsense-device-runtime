import React from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useBaseUrl from '@docusaurus/useBaseUrl';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import HomepageFeatures from '@site/src/components/HomepageFeatures';

import Heading from '@theme/Heading';
import styles from './index.module.css';

const docsIndex = [
  {
    title: 'Start here',
    description: 'Install, run a device, and validate the core flow in under five minutes.',
    to: '/docs/getting-started',
  },
  {
    title: 'Device Spec',
    description: 'Define telemetry, state, commands, and validation rules in YAML.',
    to: '/docs/device-spec/overview',
  },
  {
    title: 'Behavior & Edge Cases',
    description: 'Understand offline mode, timing, simulation loop, and gateway communication.',
    to: '/docs/behavior/simulation-loop',
  },
  {
    title: 'Device API',
    description: 'Inspect the runtime endpoints for health, state, info, and command execution.',
    to: '/docs/api/device-api',
  },
  {
    title: 'Gateway API',
    description: 'See the contract the gateway must implement for registration and telemetry.',
    to: '/docs/api/gateway-api',
  },
  {
    title: 'Contracts & Schemas',
    description: 'Browse the canonical OpenAPI and JSON Schema files used by the runtime.',
    to: '/docs/api/contracts',
  },
  {
    title: 'Testing & Docker',
    description: 'Validate contracts, wire up Docker Compose, and debug deployment issues.',
    to: '/docs/guides/testing',
  },
];

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();

  return (
    <header className={clsx('hero', styles.heroBanner)}>
      <div className="container">
        <div className={styles.heroContent}>
          <div className={styles.heroText}>
            <div className={styles.kicker}>Spec-driven IoT simulator</div>
            <div className={styles.brandRow}>
              <div className={styles.logoWrap}>
                <img
                  src={useBaseUrl('/img/logo.svg')}
                  alt="FogSense Simulator Logo"
                  width="72"
                  height="72"
                />
              </div>
              <Heading as="h1" className={styles.heroTitle}>
                {siteConfig.title}
              </Heading>
            </div>
            <p className={styles.heroSubtitle}>{siteConfig.tagline}</p>
            <div className={styles.buttons}>
              <Link className="button button--secondary button--lg" to="/docs/getting-started">
                Get Started
              </Link>
              <Link className="button button--outline button--secondary button--lg" to="/docs/">
                Architecture Overview
              </Link>
            </div>
          </div>

          <div className={styles.heroVisual}>
            <div className={styles.visualPanel}>
              <div className={styles.panelHeader}>
                <span className={styles.statusDot} />
                <span>runtime.status</span>
                <span className={styles.statusPill}>online</span>
              </div>

              <div className={styles.panelBody}>
                <div className={styles.metricRow}>
                  <span>Thermostat</span>
                  <strong>72.4°F</strong>
                </div>
                <div className={styles.metricRow}>
                  <span>Humidity</span>
                  <strong>41%</strong>
                </div>
                <div className={styles.metricRow}>
                  <span>Gateway</span>
                  <strong>healthy</strong>
                </div>

                <div className={styles.barCluster}>
                  <div className={styles.barLabel}>Telemetry health</div>
                  <div className={styles.barTrack}>
                    <span className={styles.barFill} />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </header>
  );
}

function QuickCodePreview() {
  return (
    <section className="quickPreviewSection">
      <div className="container">
        <div className="row" style={{alignItems: 'center'}}>
          <div className="col col--6 margin-bottom--lg">
            <div className="sectionEyebrow">From spec to running device</div>
            <Heading as="h2">Run a realistic device in minutes</Heading>
            <p style={{fontSize: '1.05rem', lineHeight: '1.7'}}>
              Define a device once in YAML, run it as a container, and let the runtime handle telemetry,
              state transitions, fault injection, and command execution without bespoke application code.
            </p>
            <ul style={{lineHeight: '1.9'}}>
              <li><strong>Spec-driven:</strong> no per-device Go code</li>
              <li><strong>Gateway ready:</strong> registration, heartbeat, telemetry, and events</li>
              <li><strong>Testable:</strong> works with the mock gateway and Docker Compose</li>
            </ul>
          </div>
          <div className="col col--6 margin-bottom--lg">
            <div className="previewBox">
              <div style={{color: '#94a3b8', marginBottom: '0.75rem'}}># Start a simulated light</div>
              <div style={{color: '#38bdf8'}}>$ DEVICE_ID=light-1 \
  GATEWAY_URL=http://127.0.0.1:8081 \
  ADVERTISE_URL=http://127.0.0.1:8080 \
  PORT=8080 \
  DEVICE_SPEC_FILE=examples/smart-light.yaml \
  go run .</div>
              <div style={{color: '#94a3b8', marginTop: '1rem', marginBottom: '0.75rem'}}># Send a command directly to the device</div>
              <div style={{color: '#34d399'}}>{`$ curl -X POST http://localhost:8080/commands \
  -H 'Content-Type: application/json' \
  -d '{"commandId":"demo-1","name":"SET_BRIGHTNESS","params":{"level":80}}'`}</div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

function DocumentationIndex() {
  return (
    <section className="padding-vert--xl docsIndexSection">
      <div className="container">
        <div className="text--center margin-bottom--xl">
          <div className="sectionEyebrow">Documentation hub</div>
          <Heading as="h2">Explore the runtime</Heading>
          <p style={{maxWidth: '720px', margin: '0 auto', opacity: 0.85}}>
            Start with setup, move into spec and gateway protocol, then drill into the behavior and contracts that power the platform.
          </p>
        </div>
        <div className="docIndexGrid">
          {docsIndex.map((item) => (
            <Link key={item.title} to={item.to} className="docIndexCard">
              <div className="docIndexMeta">{item.title}</div>
              <p>{item.description}</p>
              <span className="docIndexArrow">Explore →</span>
            </Link>
          ))}
        </div>
      </div>
    </section>
  );
}

export default function Home() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title={siteConfig.title}
      description="FogSense Device Runtime documentation: spec-driven generic IoT simulator.">
      <HomepageHeader />
      <main>
        <HomepageFeatures />
        <QuickCodePreview />
        <DocumentationIndex />
      </main>
    </Layout>
  );
}

import React from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useBaseUrl from '@docusaurus/useBaseUrl';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import HomepageFeatures from '@site/src/components/HomepageFeatures';

import Heading from '@theme/Heading';
import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx('hero hero--primary', styles.heroBanner)}>
      <div className="container">
        <img
          src={useBaseUrl('/img/logo.svg')}
          alt="FogSense Simulator Logo"
          width="128"
          height="128"
          style={{
            marginBottom: '1.25rem',
            filter: 'drop-shadow(0 12px 24px rgba(56, 189, 248, 0.4))',
          }}
        />
        <Heading as="h1" className="hero__title">
          {siteConfig.title}
        </Heading>
        <p className="hero__subtitle" style={{maxWidth: '720px', margin: '0 auto 1.75rem auto'}}>
          {siteConfig.tagline}
        </p>
        <div className={styles.buttons} style={{display: 'flex', gap: '1rem', justifyContent: 'center', flexWrap: 'wrap'}}>
          <Link
            className="button button--secondary button--lg"
            to="/docs/getting-started">
            Get Started (5 min) 🚀
          </Link>
          <Link
            className="button button--outline button--secondary button--lg"
            to="/docs/device-spec/overview">
            Device Spec Overview 📖
          </Link>
          <Link
            className="button button--outline button--secondary button--lg"
            to="/docs/guides/docker">
            Docker Deployment 🐳
          </Link>
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
            <Heading as="h2">Run Your First Simulated Device</Heading>
            <p style={{fontSize: '1.05rem', lineHeight: '1.6'}}>
              The FogSense Device Runtime requires zero device-specific code. Simply pass your YAML device spec via environment variable or file, connect it to your Fog Gateway, and watch telemetry stream in real time.
            </p>
            <ul style={{lineHeight: '1.8'}}>
              <li><strong>Ultra-low footprint:</strong> Under 15 MB RAM per container</li>
              <li><strong>Zero dependencies:</strong> Pure Go standalone binary</li>
              <li><strong>Docker & Compose ready:</strong> Pre-built multi-arch images</li>
            </ul>
          </div>
          <div className="col col--6 margin-bottom--lg">
            <div className="previewBox">
              <div style={{color: '#94a3b8', marginBottom: '0.75rem'}}># Start simulated smart light device</div>
              <div style={{color: '#38bdf8'}}>$ docker run --rm -d \</div>
              <div style={{paddingLeft: '1rem', color: '#e2e8f0'}}>-e DEVICE_ID=light-1 \</div>
              <div style={{paddingLeft: '1rem', color: '#e2e8f0'}}>-e GATEWAY_URL=http://gateway:8081 \</div>
              <div style={{paddingLeft: '1rem', color: '#e2e8f0'}}>-e DEVICE_SPEC_FILE=/config/smart-light.yaml \</div>
              <div style={{paddingLeft: '1rem', color: '#a78bfa'}}>fogsense-device-runtime</div>
              <div style={{color: '#94a3b8', marginTop: '1rem', marginBottom: '0.75rem'}}># Send command to device</div>
              <div style={{color: '#34d399'}}>$ curl -X POST http://localhost:8080/commands \</div>
              <div style={{paddingLeft: '1rem', color: '#e2e8f0'}}>-d '{`{"name":"SET_BRIGHTNESS","params":{"level":80}}`}'</div>
            </div>
          </div>
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
      </main>
    </Layout>
  );
}

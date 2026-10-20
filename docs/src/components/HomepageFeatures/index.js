import React from 'react';
import clsx from 'clsx';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

const FeatureList = [
  {
    title: 'Spec-Driven Simulation',
    icon: (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
        <polyline points="14 2 14 8 20 8" />
        <line x1="16" y1="13" x2="8" y2="13" />
        <line x1="16" y1="17" x2="8" y2="17" />
        <polyline points="10 9 9 9 8 9" />
      </svg>
    ),
    description: (
      <>
        Define devices entirely via YAML specifications. Telemetry metrics,
        state fields, and executable commands are generated automatically with
        zero per-device code.
      </>
    ),
  },
  {
    title: 'High-Efficiency Go Engine',
    icon: (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
      </svg>
    ),
    description: (
      <>
        Built in Go for minimal footprint: approximately 8-15 MB RAM per container
        and an ultra-compact Docker image (~15 MB). Easily spin up hundreds of simulated
        devices on a single lightweight VM.
      </>
    ),
  },
  {
    title: 'Realistic Telemetry & Noise',
    icon: (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
      </svg>
    ),
    description: (
      <>
        Simulates random-walk sensor readings, Gaussian noise, configurable
        anomalies, and state-following convergence loops (such as thermal cooling toward targets).
      </>
    ),
  },
  {
    title: 'Stateful Commands & Idempotency',
    icon: (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
        <line x1="8" y1="21" x2="16" y2="21" />
        <line x1="12" y1="17" x2="12" y2="21" />
      </svg>
    ),
    description: (
      <>
        Commands validate input parameters, apply state effects, and enforce
        strict commandId idempotency caching with instant state change notifications.
      </>
    ),
  },
  {
    title: 'Gateway Protocol & Reliability',
    icon: (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z" />
      </svg>
    ),
    description: (
      <>
        Automatic device registration, periodic heartbeats, and at-least-once
        ordered message delivery with in-memory retry queue and sequence tracking.
      </>
    ),
  },
  {
    title: 'Fault Injection & Testing',
    icon: (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
      </svg>
    ),
    description: (
      <>
        Inject faults via authenticated control APIs: simulate device disconnections,
        metric spikes, and telemetry floods. Validated against a strict mock gateway test suite.
      </>
    ),
  },
];

function FeatureCard({title, icon, description}) {
  return (
    <div className="col col--4 margin-bottom--lg">
      <div className="featureCard">
        <div className="featureIconWrapper">
          {icon}
        </div>
        <Heading as="h3" className="featureTitle">{title}</Heading>
        <p className="featureDescription">{description}</p>
      </div>
    </div>
  );
}

export default function HomepageFeatures() {
  return (
    <section className="padding-vert--xl">
      <div className="container">
        <div className="text--center margin-bottom--xl">
          <Heading as="h2">Engineered for Fog Computing Platforms</Heading>
          <p style={{maxWidth: '680px', margin: '0 auto', opacity: 0.85}}>
            Everything you need to test IoT gateways, automation rules, and dashboards with realistic simulated devices.
          </p>
        </div>
        <div className="row">
          {FeatureList.map((props, idx) => (
            <FeatureCard key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}

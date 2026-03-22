import type {ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx('hero hero--primary', styles.heroBanner)}>
      <div className="container">
        <Heading as="h1" className="hero__title">
          {siteConfig.title}
        </Heading>
        <p className="hero__subtitle">{siteConfig.tagline}</p>
        <div className={styles.buttons}>
          <Link className="button button--secondary button--lg" to="/docs/intro">
            Get Started
          </Link>
          <Link
            className="button button--outline button--lg"
            to="https://github.com/theonekeyg/boxer"
            style={{marginLeft: '1rem'}}>
            GitHub
          </Link>
        </div>
      </div>
    </header>
  );
}

type FeatureItem = {
  title: string;
  description: ReactNode;
};

const features: FeatureItem[] = [
  {
    title: 'Strong Isolation',
    description: (
      <>
        Every execution runs inside gVisor&apos;s user-space kernel, which intercepts all
        syscalls before they reach the host. Even a fully compromised container cannot
        escape to the host OS.
      </>
    ),
  },
  {
    title: 'Simple HTTP API',
    description: (
      <>
        Send a POST request with an image, command, and optional files.
        Get back stdout, stderr, exit code, and wall time. No daemon to manage,
        no sidecar containers.
      </>
    ),
  },
  {
    title: 'Any Container Image',
    description: (
      <>
        Pull any OCI image — Python, Node.js, Rust, Go, Perl — and run commands
        inside it. Images are cached locally and shared read-only across executions
        for fast startup.
      </>
    ),
  },
];

function Feature({title, description}: FeatureItem) {
  return (
    <div className={clsx('col col--4')}>
      <div className="text--center padding-horiz--md padding-vert--lg">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

export default function Home(): ReactNode {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title={siteConfig.title}
      description="Sandboxed container execution powered by gVisor">
      <HomepageHeader />
      <main>
        <section className={styles.features}>
          <div className="container">
            <div className="row">
              {features.map((props, idx) => (
                <Feature key={idx} {...props} />
              ))}
            </div>
          </div>
        </section>
        <section style={{padding: '3rem 0', background: 'var(--ifm-background-surface-color)'}}>
          <div className="container">
            <Heading as="h2" style={{textAlign: 'center', marginBottom: '2rem'}}>
              Quick Start
            </Heading>
            <div className="row">
              <div className="col col--8 col--offset-2">
                <Tabs>
                  <TabItem value="rest" label="REST API" default>
                    <pre><code>{`curl -s http://localhost:8080/run \\
  -H 'Content-Type: application/json' \\
  -d '{
    "image": "python:3.12-slim",
    "cmd": ["python3", "-c", "print(42)"]
  }'`}</code></pre>
                  </TabItem>
                  <TabItem value="python" label="Python SDK">
                    <pre><code>{`from boxer import BoxerClient

with BoxerClient("http://localhost:8080") as c:
    result = c.run(
        image="python:3.12-slim",
        cmd=["python3", "-c", "print(42)"],
    )
    print(result.stdout)  # 42`}</code></pre>
                  </TabItem>
                  <TabItem value="typescript" label="TypeScript SDK">
                    <pre><code>{`import { BoxerClient } from "boxer-sdk";

const client = new BoxerClient({
  baseUrl: "http://localhost:8080",
});
const result = await client.run(
  "python:3.12-slim",
  ["python3", "-c", "print(42)"],
);
console.log(result.stdout); // 42`}</code></pre>
                  </TabItem>
                </Tabs>
              </div>
            </div>
            <div style={{textAlign: 'center', marginTop: '2rem'}}>
              <Link className="button button--primary button--lg" to="/docs/intro">
                Read the Docs
              </Link>
            </div>
          </div>
        </section>
      </main>
    </Layout>
  );
}

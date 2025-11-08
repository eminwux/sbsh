# CI/CD Integration with sbsh

sbsh profiles enable **reproducible environments** that work identically in local development and CI/CD pipelines. Define your environment once, use it everywhere — from pre-commit hooks to GitHub Actions, GitLab CI/CD, and Jenkins pipelines.

## Key Benefits for CI/CD

- **Reproducibility**: Same profile works locally and in CI — test what you ship
- **Debugging**: Failed CI runs create persistent terminals for inspection — no more "works on my machine"
- **Version Control**: Profiles are checked into your repo — environments as code
- **Structured Logs**: Complete I/O capture available as CI artifacts — full audit trail
- **Team Consistency**: Everyone uses the same environment configuration — eliminate setup drift
- **Inspection**: Reattach to failed CI runs to debug issues interactively

## Local CI/CD Workflows

Use profiles for pre-commit hooks, local testing, and development workflows:

### Pre-commit Hook Example

```yaml
# ~/.sbsh/profiles.yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: test-env
spec:
  shell:
    cwd: "~/project"
    env:
      NODE_ENV: "test"
      CI: "true"
  stages:
    onInit:
      - script: docker-compose up -d
      - script: npm install
      - script: npm run test
```

```bash
# .git/hooks/pre-commit
#!/bin/bash
sbsh -p test-env --name "pre-commit-$(date +%s)"
```

## GitHub Actions

### Setup

Install sbsh in your GitHub Actions workflow:

```yaml
- name: Setup sbsh
  run: |
    wget -O sbsh https://github.com/eminwux/sbsh/releases/download/v0.5.1/sbsh-linux-amd64
    chmod +x sbsh && sudo mv sbsh /usr/local/bin/
    sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
```

### Complete Workflow Example

```yaml
# .github/workflows/test.yml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup sbsh
        run: |
          wget -O sbsh https://github.com/eminwux/sbsh/releases/download/v0.5.1/sbsh-linux-amd64
          chmod +x sbsh && sudo mv sbsh /usr/local/bin/
          sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb

      - name: Create test profile
        run: |
          mkdir -p ~/.sbsh
          cat > ~/.sbsh/profiles.yaml <<EOF
          apiVersion: sbsh/v1beta1
          kind: TerminalProfile
          metadata:
            name: ci-test
          spec:
            shell:
              cwd: "${{ github.workspace }}"
              env:
                CI: "true"
                NODE_ENV: "test"
            stages:
              onInit:
                - script: npm install
                - script: npm run test
          EOF

      - name: Run tests
        run: sbsh -p ci-test --name "ci-${{ github.run_id }}"

      - name: Upload terminal logs
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: terminal-logs-${{ github.run_id }}
          path: ~/.sbsh/run/terminals/ci-${{ github.run_id }}/*
          retention-days: 7
```

### Best Practices for GitHub Actions

- **Use unique terminal names**: Include `github.run_id` or `github.run_number` to avoid conflicts
- **Upload artifacts on failure**: Capture terminal logs for debugging failed runs
- **Set retention**: Limit artifact retention to manage storage costs
- **Use profiles from repo**: Consider checking in profiles to your repository for version control
- **Reusable workflows**: Create reusable workflows for common test patterns

### Using Profiles from Repository

Instead of creating profiles inline, you can use profiles checked into your repository:

```yaml
- name: Copy profiles from repository
  run: |
    mkdir -p ~/.sbsh
    cp .github/profiles/ci-test.yaml ~/.sbsh/profiles.yaml
    # Or combine multiple profiles
    cat .github/profiles/*.yaml > ~/.sbsh/profiles.yaml
```

## GitLab CI/CD

### Setup

Install sbsh in your GitLab CI pipeline:

```yaml
before_script:
  - |
    if ! command -v sbsh &> /dev/null; then
      wget -O sbsh https://github.com/eminwux/sbsh/releases/download/v0.5.1/sbsh-linux-amd64
      chmod +x sbsh && sudo mv sbsh /usr/local/bin/
      sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
    fi
```

### Complete Pipeline Example

```yaml
# .gitlab-ci.yml
test:
  image: ubuntu:latest
  before_script:
    - apt-get update && apt-get install -y wget procps
    - |
      wget -O sbsh https://github.com/eminwux/sbsh/releases/download/v0.5.1/sbsh-linux-amd64
      chmod +x sbsh && sudo mv sbsh /usr/local/bin/
      sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
  script:
    - |
      mkdir -p ~/.sbsh
      cat > ~/.sbsh/profiles.yaml <<EOF
      apiVersion: sbsh/v1beta1
      kind: TerminalProfile
      metadata:
        name: gitlab-test
      spec:
        shell:
          cwd: "$CI_PROJECT_DIR"
          env:
            CI_JOB_ID: "$CI_JOB_ID"
            CI_PROJECT_NAME: "$CI_PROJECT_NAME"
        stages:
          onInit:
            - script: npm install
            - script: npm run test
      EOF
    - sbsh -p gitlab-test --name "gitlab-$CI_JOB_ID"
  artifacts:
    when: always
    paths:
      - ~/.sbsh/run/terminals/gitlab-$CI_JOB_ID/*
    expire_in: 1 week
```

### Best Practices for GitLab CI/CD

- **Use CI_JOB_ID**: Unique job IDs ensure terminal names don't conflict
- **Artifacts on always**: Use `when: always` to capture logs even on success (useful for debugging)
- **Set expiration**: Use `expire_in` to manage artifact storage
- **Use GitLab variables**: Leverage `$CI_PROJECT_DIR`, `$CI_COMMIT_REF_NAME`, etc. for dynamic configuration
- **Separate profiles**: Store profiles in `.gitlab/profiles/` directory for organization

### Using Profiles from Repository

```yaml
test:
  script:
    - mkdir -p ~/.sbsh
    - cp .gitlab/profiles/ci-test.yaml ~/.sbsh/profiles.yaml
    - sbsh -p ci-test --name "gitlab-$CI_JOB_ID"
```

## Jenkins

### Setup

Install sbsh in your Jenkins pipeline:

```groovy
sh '''
  wget -O sbsh https://github.com/eminwux/sbsh/releases/download/v0.5.1/sbsh-linux-amd64
  chmod +x sbsh && sudo mv sbsh /usr/local/bin/
  sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
'''
```

### Declarative Pipeline Example

```groovy
// Jenkinsfile
pipeline {
    agent any

    environment {
        SBSH_VERSION = 'v0.5.1'
    }

    stages {
        stage('Setup') {
            steps {
                sh '''
                    wget -O sbsh https://github.com/eminwux/sbsh/releases/download/${SBSH_VERSION}/sbsh-linux-amd64
                    chmod +x sbsh && sudo mv sbsh /usr/local/bin/
                    sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
                '''
            }
        }

        stage('Create Profile') {
            steps {
                sh '''
                    mkdir -p ~/.sbsh
                    cat > ~/.sbsh/profiles.yaml <<EOF
                    apiVersion: sbsh/v1beta1
                    kind: TerminalProfile
                    metadata:
                      name: jenkins-test
                    spec:
                      shell:
                        cwd: "${WORKSPACE}"
                        env:
                          BUILD_NUMBER: "${BUILD_NUMBER}"
                          JOB_NAME: "${JOB_NAME}"
                      stages:
                        onInit:
                          - script: npm install
                          - script: npm run test
                    EOF
                '''
            }
        }

        stage('Run Tests') {
            steps {
                sh 'sbsh -p jenkins-test --name "jenkins-${BUILD_NUMBER}"'
            }
        }
    }

    post {
        always {
            archiveArtifacts artifacts: '~/.sbsh/run/terminals/jenkins-*/**', allowEmptyArchive: true
        }
        failure {
            echo 'Tests failed. Check terminal logs in archived artifacts.'
        }
    }
}
```

### Scripted Pipeline Example

```groovy
// Jenkinsfile (Scripted)
node {
    def buildNumber = env.BUILD_NUMBER

    stage('Setup') {
        sh '''
            wget -O sbsh https://github.com/eminwux/sbsh/releases/download/v0.5.1/sbsh-linux-amd64
            chmod +x sbsh && sudo mv sbsh /usr/local/bin/
            sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
        '''
    }

    stage('Create Profile') {
        sh """
            mkdir -p ~/.sbsh
            cat > ~/.sbsh/profiles.yaml <<EOF
            apiVersion: sbsh/v1beta1
            kind: TerminalProfile
            metadata:
              name: jenkins-test
            spec:
              shell:
                cwd: "\${WORKSPACE}"
                env:
                  BUILD_NUMBER: "${buildNumber}"
                  JOB_NAME: "\${JOB_NAME}"
              stages:
                onInit:
                  - script: npm install
                  - script: npm run test
            EOF
        """
    }

    stage('Run Tests') {
        sh "sbsh -p jenkins-test --name jenkins-${buildNumber}"
    }

    stage('Archive Artifacts') {
        archiveArtifacts artifacts: "~/.sbsh/run/terminals/jenkins-${buildNumber}/**", allowEmptyArchive: true
    }
}
```

### Best Practices for Jenkins

- **Use BUILD_NUMBER**: Ensure unique terminal names across builds
- **Archive artifacts**: Use `archiveArtifacts` to preserve terminal logs
- **Post actions**: Use `post { always { } }` to archive even on failure
- **Workspace variables**: Use `$WORKSPACE` and other Jenkins environment variables
- **Version pinning**: Store sbsh version in environment variable for easy updates

### Using Profiles from Repository

```groovy
stage('Copy Profiles') {
    steps {
        sh '''
            mkdir -p ~/.sbsh
            cp jenkins/profiles/ci-test.yaml ~/.sbsh/profiles.yaml
        '''
    }
}
```

## Best Practices (All Platforms)

### Profile Management

- **Version control profiles**: Store profiles in your repository (`.github/profiles/`, `.gitlab/profiles/`, `jenkins/profiles/`)
- **Reuse local profiles**: Same profiles work locally and in CI — test locally first
- **Profile naming**: Use descriptive names like `ci-test`, `ci-lint`, `ci-build`
- **Environment variables**: Use CI-specific variables for dynamic configuration

### Terminal Naming

- **Unique names**: Always include unique identifiers (run ID, job ID, build number)
- **Descriptive names**: Include job type for easier identification: `ci-test-123`, `ci-build-456`
- **Avoid conflicts**: Never reuse terminal names across concurrent runs

### Artifact Management

- **Upload on failure**: Capture logs when tests fail for debugging
- **Set retention**: Limit artifact retention to manage storage costs
- **Include metadata**: Terminal metadata files contain valuable debugging information
- **Compress if needed**: Consider compressing large artifacts

### Debugging Failed Runs

When a CI run fails, you can:

1. **Download artifacts**: Terminal logs are available in CI artifacts
2. **Inspect logs**: Check `~/.sbsh/run/terminals/<name>/capture.log` for full I/O
3. **Check metadata**: Review `meta.json` for terminal state and environment
4. **Recreate locally**: Use the same profile locally to reproduce issues

### Common Patterns

**Multi-stage builds:**

```yaml
stages:
  onInit:
    - script: docker build -t myapp .
    - script: docker run -d --name test myapp
    - script: npm run test
```

**Environment-specific profiles:**

```yaml
env:
  ENVIRONMENT: "production"
  DATABASE_URL: "${{ secrets.PROD_DATABASE_URL }}"
```

**Parallel test execution:**

```yaml
# GitHub Actions
jobs:
  test:
    strategy:
      matrix:
        test-suite: [unit, integration, e2e]
    steps:
      - run: sbsh -p ci-test-${{ matrix.test-suite }} --name "ci-${{ github.run_id }}-${{ matrix.test-suite }}"
```

## Troubleshooting

### Profile Not Found

**Problem**: `sbsh` says "profile not found" in CI

**Solutions**:

- Verify profile file path: `~/.sbsh/profiles.yaml` by default
- Check profile name matches exactly (case-sensitive)
- Ensure profile is created before running `sbsh -p`
- Use `sb get profiles` to list available profiles

### Terminal Not Starting

**Problem**: Terminal fails to start in CI

**Solutions**:

- Check working directory exists: `cwd` must be valid
- Verify commands in `onInit` are available in CI environment
- Check file permissions and paths
- Review terminal logs in artifacts

### Artifacts Not Uploaded

**Problem**: Terminal logs not available after CI run

**Solutions**:

- Verify artifact paths match terminal name pattern
- Use `when: always` (GitLab) or `if: always()` (GitHub Actions) to capture on success
- Check artifact retention settings
- Ensure terminal completed (check exit codes)

### Environment Variables Not Set

**Problem**: Environment variables not available in terminal

**Solutions**:

- Use CI-specific variable syntax: `${{ github.workspace }}`, `$CI_PROJECT_DIR`, `$WORKSPACE`
- Check `inheritEnv` setting in profile
- Verify variable expansion in profile YAML
- Use absolute paths if variable expansion fails

## See Also

- [Profiles Documentation](../profiles/README.md) - Learn how to create and customize profiles
- [Main README](../README.md) - Overview of sbsh features and capabilities

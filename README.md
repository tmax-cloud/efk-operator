# efk-operator

## Prerequisite
  * [hyperauth](https://github.com/tmax-cloud/install-hyperauth/tree/5.0)  
  * [hypercloud-api-server](https://github.com/tmax-cloud/install-hypercloud/tree/5.0)

## Step 0. kustomize 및 적용
  * 목적 : `yaml 파일 생성 및 적용을 위한 kustomize 실행`
  * 순서 :
    ``` bash
    $ kustomize build config/default/ | kubectl apply -f -
    ```
  * 비고 :
    * [config/manager/kustomization.yaml](https://github.com/tmax-cloud/efk-operator/blob/master/config/manager/kustomization.yaml) 에서 이미지 버전 지정
    * kube-logging 네임스페이스가 없다면 직접 생성해야 함

## Flow
  1. 유저가 fluentbitconfiguration 리소스를 생성
  2. efk-operator가 이를 감지하고 그에 해당하는 configmap 생성
  3. injection 하고싶은 리소스에 아래와 같이 두 가지 label을 추가
  * log-collector: enbaled
  * tmax.io/log-collector-configuration: {생성된 configmap 이름}
  4. 해당 리소스 생성시, [hypercloud-api-server의 webhook](https://github.com/tmax-cloud/hypercloud-api-server/blob/master/admission/sidecarInjectionHandler.go) 의해 sidecar injection 됨

## 예시
  * fluentbitconfiguration
    ``` bash
    apiVersion: config.tmax.io/v1alpha1
    kind: FluentBitConfiguration
    metadata:
      name: fbc-test
      namespace: kube-logging
    spec:
      inputPlugins:
      - path: /test/log/
        pattern: "*.log"
        tag: testtag
      filterPlugins:
      - parserName: parser
        regex: .*
        tag: testtag
      outputPlugins:
      - indexName: testtag_prefix
        tag: testtag
    ```
  * injection for pod
    ``` bash
    apiVersion: v1
    kind: Pod
    metadata:
      name: counter
      namespace: kube-logging
      labels:
        log-collector: enabled
        tmax.io/log-collector-configuration: fbc-test
    spec:
      containers:
      - name: count
        image: busybox
        imagePullPolicy: IfNotPresent
        args: [/bin/sh, -c, 'i=0; while true; do echo "$(date)" && sleep 10; i=$(i+1)); done']
        env:
        - name: FLUENT_ELASTICSEARCH_HOST
          value: "elasticsearch.kube-logging.svc.cluster.local"
        - name: FLUENT_ELASTICSEARCH_PORT
          value: "9200"
        - name: FLUENT_ELASTICSEARCH_SCHEME
          value: "http" 
    ```
  * injection for deployment
    ``` bash
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: counter-deploy
      namespace: kube-logging
      labels:
        log-collector: enabled
        tmax.io/log-collector-configuration: fbc-test
    spec:
      replicas: 1
      selector:
        matchLabels:
          kube-logging: injection-test
      template:
        metadata:
          name: counter
          namespace: kube-logging
          labels:
            kube-logging: injection-test
        spec:
          containers:
          - name: count
            image: busybox
            imagePullPolicy: IfNotPresent
            args: [/bin/sh, -c, 'i=0; while true; do echo "$(date)" && sleep 10; i=$((i+1)); done']
            env:
            - name: FLUENT_ELASTICSEARCH_HOST
              value: "elasticsearch.kube-logging.svc.cluster.local"
            - name: FLUENT_ELASTICSEARCH_PORT
              value: "9200"
            - name: FLUENT_ELASTICSEARCH_SCHEME
              value: "http"
    ```

## 참고
  * [MutatingWebhookConfiguration](https://github.com/tmax-cloud/install-hypercloud/blob/5.0/hypercloud-api-server/config/webhook-configuration.yaml)
    * caBundle에는 openssl base64 -A < "/etc/kubernetes/pki/hypercloud-root-ca.crt" 값 

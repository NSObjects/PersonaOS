import { PageContainer } from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { Alert, Button, Card, Checkbox, Form, Input, Select, Space, Spin, Tag, Typography, message } from 'antd';
import React, { useEffect, useState } from 'react';

import { listProviderConfigs, type ProviderConfig, updateProviderConfig } from '@/services/admin';

type Field = { name: string; label: string; secret?: boolean; placeholder?: string; advanced?: boolean };
type Group = { id: string; title: string; description?: string; fields: Field[] };

const groups: Group[] = [
  {
    id: 'volcengine', title: '火山引擎', description: '豆包语音与火山方舟使用独立凭据。',
    fields: [
      { name: 'speech_api_key', label: '豆包语音 API Key', secret: true },
      { name: 'ark_api_key', label: '火山方舟 API Key（Embedding）', secret: true },
      { name: 'asr_resource_id', label: 'ASR Resource ID', placeholder: '留空使用 SDK 默认值', advanced: true },
      { name: 'tts_model_id', label: 'TTS Model ID', placeholder: '留空使用 SDK 默认值', advanced: true },
    ],
  },
  {
    id: 'aliyun', title: '阿里云 · DashScope',
    fields: [{ name: 'api_key', label: 'API Key', secret: true }],
  },
  {
    id: 'tencent', title: '腾讯云',
    fields: [
      { name: 'app_id', label: 'App ID' },
      { name: 'secret_id', label: 'Secret ID', secret: true },
      { name: 'secret_key', label: 'Secret Key', secret: true },
    ],
  },
  {
    id: 'deepseek', title: 'DeepSeek', description: '官方网页搜索',
    fields: [{ name: 'api_key', label: 'API Key', secret: true }],
  },
  {
    id: 'llm', title: 'OpenAI 兼容模型服务',
    fields: [
      { name: 'api_key', label: 'API Key', secret: true },
      { name: 'model', label: 'Model' },
      { name: 'base_url', label: 'Base URL（可选）', placeholder: '使用默认服务地址' },
    ],
  },
];

const asrGroup: Group = { id: 'asr', title: '语音识别', fields: [{ name: 'provider', label: '默认 ASR 厂商' }] };

const ProviderCard: React.FC<{
  group: Group;
  config: ProviderConfig;
  editable: boolean;
  onSaved: (config: ProviderConfig) => void;
}> = ({ group, config, editable, onSaved }) => {
  const [form] = Form.useForm<Record<string, string>>();
  const [clear, setClear] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [advanced, setAdvanced] = useState(false);
  useEffect(() => {
    form.resetFields();
    form.setFieldsValue(config.values);
    setClear([]);
  }, [config, form]);

  const save = async (values: Record<string, string>) => {
    const changes: Record<string, string> = {};
    for (const field of group.fields) {
      const value = values[field.name] ?? '';
      if (field.secret) {
        if (clear.includes(field.name)) changes[field.name] = '';
        else if (value.trim()) changes[field.name] = value;
      } else if (value !== (config.values[field.name] ?? '')) {
        changes[field.name] = value;
      }
    }
    if (Object.keys(changes).length === 0) return;
    setSaving(true);
    try {
      const next = await updateProviderConfig(group.id, changes);
      form.resetFields();
      setClear([]);
      onSaved(next);
      message.success('配置已保存');
    } catch {
      message.error('保存失败，请检查配置');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card title={group.title} extra={<Tag>{config.source === 'database' ? '平台配置' : '环境变量'}</Tag>}>
      {group.description && <Typography.Paragraph type="secondary">{group.description}</Typography.Paragraph>}
      <Form form={form} layout="vertical" disabled={!editable || saving} onFinish={save} initialValues={config.values}>
        {group.fields.map((field) => (
          <div key={field.name} style={{ display: field.advanced && !advanced ? 'none' : undefined }}>
            {field.secret ? (
              <>
                <Form.Item name={field.name} label={field.label} style={{ marginBottom: 8 }}>
                  <Input.Password
                    autoComplete="new-password"
                    maxLength={4096}
                    disabled={clear.includes(field.name)}
                    placeholder={config.secrets[field.name] ? '已配置，留空不修改' : '未配置'}
                  />
                </Form.Item>
                <Checkbox
                  checked={clear.includes(field.name)}
                  disabled={!editable || saving || !config.secrets[field.name]}
                  onChange={(event) => {
                    setClear((old) => event.target.checked ? [...old, field.name] : old.filter((name) => name !== field.name));
                    if (event.target.checked) form.setFieldValue(field.name, '');
                  }}
                  style={{ marginBottom: 16 }}
                >清空此凭据</Checkbox>
              </>
            ) : (
              <Form.Item name={field.name} label={field.label}>
                {field.name === 'provider' ? (
                  <Select options={[
                    { value: '', label: '未启用' },
                    { value: 'volcengine', label: '火山引擎' },
                    { value: 'aliyun', label: '阿里云' },
                    { value: 'tencent', label: '腾讯云' },
                  ]} />
                ) : <Input maxLength={4096} placeholder={field.placeholder} />}
              </Form.Item>
            )}
          </div>
        ))}
        <Space>
          <Button type="primary" htmlType="submit" disabled={!editable} loading={saving}>保存配置</Button>
          {group.fields.some((field) => field.advanced) && (
            <Button type="link" onClick={() => setAdvanced(!advanced)}>{advanced ? '收起高级设置' : '高级设置'}</Button>
          )}
        </Space>
      </Form>
    </Card>
  );
};

const ProviderConfigs: React.FC = () => {
  const access = useAccess();
  const [configs, setConfigs] = useState<ProviderConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    void listProviderConfigs().then((data) => {
      if (active) setConfigs(data);
    }).catch(() => {
      if (active) setFailed(true);
    }).finally(() => {
      if (active) setLoading(false);
    });
    return () => { active = false; };
  }, []);
  const saved = (config: ProviderConfig) => setConfigs((old) => old.map((item) => item.id === config.id ? config : item));
  const render = (group: Group) => {
    const config = configs.find((item) => item.id === group.id);
    return config ? <ProviderCard key={group.id} group={group} config={config} editable={access.canProviderConfigUpdate} onSaved={saved} /> : null;
  };
  return (
    <PageContainer title="第三方服务配置">
      <Spin spinning={loading}>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          {failed && <Alert type="error" showIcon message="读取第三方服务配置失败" />}
          <Alert type="info" showIcon message="密钥不会回显；留空保留现有密钥。保存后新请求使用最新配置。" />
          <Typography.Title level={4}>服务凭据</Typography.Title>
          {groups.map(render)}
          <Typography.Title level={4}>运行配置</Typography.Title>
          {render(asrGroup)}
        </Space>
      </Spin>
    </PageContainer>
  );
};

export default ProviderConfigs;

import { PlusOutlined } from '@ant-design/icons';
import {
  type ActionType, ModalForm, PageContainer, type ProColumns, ProFormDependency, ProFormSelect,
  ProFormSwitch, ProFormText, ProFormTextArea, ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { Alert, Button, Form, message, Popconfirm, Select, Tag, Upload } from 'antd';
import React, { useEffect, useRef, useState } from 'react';
import {
  type CapabilityResource, type CapabilityResourceInput, type CapabilityResourceKind, type RuntimeResourceOptions,
  createCapabilityResource, deleteCapabilityResource, getRuntimeResourceOptions, listCapabilityResources, updateCapabilityResource,
} from '@/services/admin';

import KnowledgeInspection from './KnowledgeInspection';
import VoicePreview from './VoicePreview';

const kindLabels: Record<CapabilityResourceKind, string> = {
  voice: '音色', tool: 'Tool', skill: 'Skill', knowledge_base: '知识库',
};
const kinds = (Object.keys(kindLabels) as CapabilityResourceKind[]).map(
  (kind) => ({ label: kindLabels[kind], value: kind }),
);

const providerLabels: Record<string, string> = { aliyun: '阿里云', volcengine: '火山引擎', tencent: '腾讯云', builtin: '内置 Tool', http: 'HTTP Tool' };
const referenceLabels: Record<string, string> = { get_current_time: '当前时间', get_weather: '天气查询', web_search: '网络搜索', storytelling: '故事讲述' };

function defaultProvider(kind: CapabilityResourceKind): string {
  if (kind === 'tool') return 'builtin';
  if (kind === 'knowledge_base') return 'redis';
  return '';
}

function preserveCurrent(options: { label: string; value: string }[], current?: string) {
  return current && !options.some((item) => item.value === current)
    ? [...options, { label: `${current}（当前配置，未注册或不可选）`, value: current, disabled: true }]
    : options;
}

const CapabilityResources: React.FC = () => {
  const access = useAccess();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [kind, setKind] = useState<CapabilityResourceKind>('voice');
  const [editing, setEditing] = useState<CapabilityResource>();
  const [open, setOpen] = useState(false);
  const [knowledgeFile, setKnowledgeFile] = useState<File>();
  const [inspecting, setInspecting] = useState<CapabilityResource>();
  const [previewing, setPreviewing] = useState<CapabilityResource>();
  const [form] = Form.useForm<CapabilityResourceInput>();
  const [runtimeOptions, setRuntimeOptions] = useState<RuntimeResourceOptions>();
  const [optionsFailed, setOptionsFailed] = useState(false);

  useEffect(() => {
    if (!open) return;
    let active = true;
    setRuntimeOptions(undefined);
    setOptionsFailed(false);
    void getRuntimeResourceOptions().then((options) => {
      if (active) setRuntimeOptions(options);
    }).catch(() => { if (active) setOptionsFailed(true); });
    return () => { active = false; };
  }, [open]);

  const columns: ProColumns<CapabilityResource>[] = [
    { title: '名称', dataIndex: 'name' },
    { title: '编码', dataIndex: 'code' },
    { title: '类型', dataIndex: 'kind', render: (_, item) => kindLabels[item.kind] },
    { title: '厂商', dataIndex: 'provider', search: false },
    { title: '引用键', dataIndex: 'reference', ellipsis: true, search: false },
    { title: '启用', dataIndex: 'enabled', search: false, render: (_, item) => (
      <Tag color={item.enabled ? 'success' : 'default'}>{item.enabled ? '已启用' : '已停用'}</Tag>
    ) },
    {
      title: '操作', valueType: 'option',
      render: (_, item) => [
        ...(item.kind === 'voice' ? [
          <Button key="preview" type="link" size="small" disabled={!item.enabled} onClick={() => setPreviewing(item)}>试听</Button>,
        ] : []),
        ...(item.kind === 'knowledge_base' && item.provider === 'redis' ? [
          <Button key="inspect" type="link" size="small" onClick={() => setInspecting(item)}>索引与检索</Button>,
        ] : []),
        ...(access.canCapabilityResourceUpdate ? [
          <a key="edit" onClick={() => { setEditing(item); setOpen(true); }}>编辑</a>,
        ] : []),
        ...(access.canCapabilityResourceDelete ? [
          <Popconfirm key="delete" title="删除能力资源"
            description={'确认删除 ' + item.name + '？被设备类型引用的资源不可删除。'}
            onConfirm={async () => {
              await deleteCapabilityResource(item.id);
              message.success('资源已删除');
              actionRef.current?.reload();
            }}>
            <Button type="link" danger size="small">删除</Button>
          </Popconfirm>,
        ] : []),
      ],
    },
  ];

  return (
    <PageContainer title="能力资源" subTitle="维护可复用资源，设备类型独立选择；此处不发布设备配置">
      <ProTable<CapabilityResource> key={kind} rowKey="id" actionRef={actionRef} columns={columns}
        search={false} pagination={false}
        request={async () => {
          const data = await listCapabilityResources(kind);
          return { data, success: true, total: data.length };
        }}
        toolBarRender={() => [
          <Select<CapabilityResourceKind> key="kind" aria-label="资源类型" style={{ width: 145 }} value={kind}
            options={kinds} onChange={(next) => setKind(next)} />,
          ...(access.canCapabilityResourceCreate ? [
            <Button key="create" type="primary" icon={<PlusOutlined />}
              onClick={() => { setEditing(undefined); setOpen(true); }}>新增资源</Button>,
          ] : []),
        ]}
      />
      {previewing && <VoicePreview key={previewing.id} resource={previewing} onClose={() => setPreviewing(undefined)} />}
      {inspecting && <KnowledgeInspection key={inspecting.id} resource={inspecting} onClose={() => setInspecting(undefined)} />}
      <ModalForm<CapabilityResourceInput> key={editing?.id ?? 'create'} open={open}
        form={form} title={editing ? '编辑能力资源' : '新增能力资源'}
        onOpenChange={(next) => { setOpen(next); if (!next) setKnowledgeFile(undefined); }} modalProps={{ destroyOnHidden: true }}
        initialValues={editing ?? {
          kind, code: '', name: '', reference: '', provider: defaultProvider(kind), description: '', enabled: true,
        }}
        onFinish={async (values) => {
          const resourceKind = editing?.kind ?? values.kind;
          const data = { ...values, tool_config: resourceKind === 'tool' && values.provider === 'http' ? values.tool_config : '' };
          if (resourceKind === 'skill') data.provider = '';
          if (resourceKind === 'knowledge_base') {
            data.provider = 'redis';
            if (knowledgeFile) {
              try {
                data.knowledge_text = new TextDecoder('utf-8', { fatal: true }).decode(await knowledgeFile.arrayBuffer());
              } catch {
                message.error('请选择有效的 UTF-8 文档');
                return false;
              }
              data.knowledge_filename = knowledgeFile.name;
            }
          }
          if (editing) {
            await updateCapabilityResource(editing.id, data);
            message.success('资源已更新');
          } else {
            await createCapabilityResource(data);
            message.success('资源已创建');
          }
          if (resourceKind && resourceKind !== kind) setKind(resourceKind);
          else actionRef.current?.reload();
          return true;
        }}>
        {optionsFailed && <Alert type="error" showIcon message="能力选项加载失败，请检查后台授权并重新打开。" />}
        <ProFormSelect name="kind" label="资源类型" options={kinds}
          fieldProps={{ onChange: (next) => {
            if (next !== "voice" && next !== "tool" && next !== "skill" && next !== "knowledge_base") return;
            form.setFieldsValue({ provider: defaultProvider(next), reference: '', tool_config: '' });
            setKnowledgeFile(undefined);
          } }} disabled={Boolean(editing)} rules={[{ required: true, message: '请选择资源类型' }]} />
        <ProFormText name="code" label="唯一编码" disabled={Boolean(editing)}
          rules={[{ required: true, message: '请输入资源编码' },
            { pattern: /^[a-zA-Z][a-zA-Z0-9_-]{1,63}$/, message: '2–64 位，英文开头，仅支持字母、数字、下划线、连字符' }]} />
        <ProFormText name="name" label="资源名称" fieldProps={{ maxLength: 160 }}
          rules={[{ required: true, message: '请输入资源名称' }]} />
        <ProFormDependency name={['kind']}>
          {({ kind: resourceKind }) => {
            const type = editing?.kind ?? resourceKind;
            if (type !== 'voice' && type !== 'tool') return null;
            const providers = type === 'voice' ? (runtimeOptions?.voice_providers ?? []) : ['builtin', 'http'];
            return <ProFormSelect name="provider" label={type === 'voice' ? '语音厂商' : 'Tool 类型'}
              options={preserveCurrent(providers.map((value) => ({ value, label: providerLabels[value] ?? value })), editing?.provider || undefined)}
              fieldProps={{ loading: !runtimeOptions && !optionsFailed, onChange: () => form.setFieldsValue({ reference: '', tool_config: '' }) }}
              rules={[{ required: true, message: '请选择厂商或 Tool 类型' }]} />;
          }}
        </ProFormDependency>
        <ProFormDependency name={['kind', 'provider']}>
          {({ kind: resourceKind, provider }) => {
            const type = editing?.kind ?? resourceKind;
            if (type === 'tool' || type === 'skill') {
              const choices = type === 'skill' ? runtimeOptions?.skills ?? [] :
                (runtimeOptions?.tools ?? []).filter((item) => item.provider === (provider || 'builtin'));
              const options = choices.map((item) => ({ value: item.reference,
                label: `${referenceLabels[item.reference] ?? item.reference} · ${item.reference}` }));
              return <ProFormSelect name="reference" label={type === 'skill' ? '已安装 Skill' : '执行器'}
                options={preserveCurrent(options, editing?.provider === provider || type === 'skill' ? editing?.reference : undefined)}
                fieldProps={{ loading: !runtimeOptions && !optionsFailed, showSearch: true,
                  onChange: (reference) => {
                    if (typeof reference !== 'string') return;
                    const selected = choices.find((item) => item.reference === reference);
                    if (!selected) return;
                    if (!form.getFieldValue('name')) form.setFieldValue('name', referenceLabels[reference] ?? reference);
                    if (!editing && !form.getFieldValue('code') && /^[a-z][a-z0-9_-]{1,63}$/.test(reference)) form.setFieldValue('code', reference);
                    if (!form.getFieldValue('description')) form.setFieldValue('description', selected.description ?? '');
                  } }}
                rules={[{ required: true, message: '请选择已注册的执行器' }]} />;
            }
            const isKnowledge = type === 'knowledge_base';
            return <>
              {isKnowledge && <Form.Item label="知识文档" extra="TXT / Markdown，UTF-8，最多 64 KiB。上传后构建新版本；设备类型需重新应用配置。">
                <Upload accept=".txt,.md,.markdown" maxCount={1}
                  beforeUpload={(file) => {
                    if (file.size > 64 * 1024 || !/\.(txt|md|markdown)$/i.test(file.name)) {
                      message.error('仅支持不超过 64 KiB 的 TXT 或 Markdown 文件');
                      return Upload.LIST_IGNORE;
                    }
                    setKnowledgeFile(file);
                    return false;
                  }}
                  onRemove={() => { setKnowledgeFile(undefined); }}
                  fileList={knowledgeFile ? [{ uid: 'knowledge-source', name: knowledgeFile.name }] : []}>
                  <Button>选择知识文档</Button>
                </Upload>
              </Form.Item>}
              <ProFormText name="reference" label={isKnowledge ? '知识版本' : '音色 ID'}
                fieldProps={{ maxLength: 512, disabled: isKnowledge && Boolean(knowledgeFile) }}
                tooltip={isKnowledge ? '上传文档时自动生成新版本；也可引用已索引版本' : '仅填写已登记的引用标识，禁止存放密钥、Token、密码或脚本'}
                rules={[{ required: !(isKnowledge && knowledgeFile), message: '请输入音色 ID 或知识版本' }]} />
            </>;
          }}
        </ProFormDependency>
        <ProFormDependency name={['kind', 'provider']}>
          {({ kind: resourceKind, provider }) => resourceKind === 'tool' && provider === 'http' ? (
            <ProFormTextArea name="tool_config" label="HTTP Tool 配置" fieldProps={{ rows: 7, maxLength: 8192 }}
              placeholder={'{"method":"GET","input_schema":{"type":"object","properties":{"city":{"type":"string","description":"城市"}},"required":["city"]}}'}
              rules={[{ required: true, message: '请输入 HTTP 方法和参数 Schema' }, {
                validator: async (_: unknown, value: string | undefined) => {
                  if (value) {
                    try {
                      JSON.parse(value);
                    } catch {
                      throw new Error('请输入有效 JSON');
                    }
                  }
                },
                message: '请输入有效 JSON',
              }]} />
          ) : null}
        </ProFormDependency>
        <ProFormTextArea name="description" label="说明" fieldProps={{ rows: 3, maxLength: 2000 }} />
        <ProFormSwitch name="enabled" label="启用" />
      </ModalForm>
    </PageContainer>
  );
};

export default CapabilityResources;

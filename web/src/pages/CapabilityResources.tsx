import { PlusOutlined } from '@ant-design/icons';
import {
  type ActionType, ModalForm, PageContainer, type ProColumns, ProFormDependency, ProFormSelect,
  ProFormSwitch, ProFormText, ProFormTextArea, ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { Button, Form, message, Popconfirm, Select, Tag, Upload } from 'antd';
import React, { useRef, useState } from 'react';
import {
  type CapabilityResource, type CapabilityResourceInput, type CapabilityResourceKind,
  createCapabilityResource, deleteCapabilityResource, listCapabilityResources, updateCapabilityResource,
} from '@/services/admin';

const kindLabels: Record<CapabilityResourceKind, string> = {
  voice: '音色', tool: 'Tool', skill: 'Skill', knowledge_base: '知识库',
};
const kinds = (Object.keys(kindLabels) as CapabilityResourceKind[]).map(
  (kind) => ({ label: kindLabels[kind], value: kind }),
);

const CapabilityResources: React.FC = () => {
  const access = useAccess();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [kind, setKind] = useState<CapabilityResourceKind>('voice');
  const [editing, setEditing] = useState<CapabilityResource>();
  const [open, setOpen] = useState(false);
  const [knowledgeFile, setKnowledgeFile] = useState<File>();

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
      <ModalForm<CapabilityResourceInput> key={editing?.id ?? 'create'} open={open}
        title={editing ? '编辑能力资源' : '新增能力资源'}
        onOpenChange={(next) => { setOpen(next); if (!next) setKnowledgeFile(undefined); }} modalProps={{ destroyOnHidden: true }}
        initialValues={editing ?? {
          kind, code: '', name: '', reference: '', provider: kind === 'knowledge_base' ? 'redis' : '', description: '', enabled: true,
        }}
        onFinish={async (values) => {
          const resourceKind = editing?.kind ?? values.kind;
          const data = { ...values, tool_config: resourceKind === 'tool' && values.provider === 'http' ? values.tool_config : '' };
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
          actionRef.current?.reload();
          return true;
        }}>
        <ProFormSelect name="kind" label="资源类型" options={kinds}
          disabled={Boolean(editing)} rules={[{ required: true, message: '请选择资源类型' }]} />
        <ProFormText name="code" label="唯一编码" disabled={Boolean(editing)}
          rules={[{ required: true, message: '请输入资源编码' },
            { pattern: /^[a-zA-Z][a-zA-Z0-9_-]{1,63}$/, message: '2–64 位，英文开头，仅支持字母、数字、下划线、连字符' }]} />
        <ProFormText name="name" label="资源名称" fieldProps={{ maxLength: 160 }}
          rules={[{ required: true, message: '请输入资源名称' }]} />
        <ProFormText name="provider" label="厂商（音色必填）" fieldProps={{ maxLength: 120 }}
          tooltip="Tool 类型填写 builtin 或 http；HTTP 引用键为服务端批准的端点标识" />
        <ProFormDependency name={['kind']}>
          {({ kind: resourceKind }) => {
            const isKnowledge = (editing?.kind ?? resourceKind) === 'knowledge_base';
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
              <ProFormText name="reference" label={isKnowledge ? '知识版本' : '资源引用键'}
                fieldProps={{ maxLength: 512, disabled: isKnowledge && Boolean(knowledgeFile) }}
                tooltip={isKnowledge ? '上传文档时自动生成新版本；也可引用已索引版本' : '仅填写已登记的引用标识，禁止存放密钥、Token、密码或脚本'}
                rules={[{ required: !(isKnowledge && knowledgeFile), message: '请输入资源引用键' }]} />
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

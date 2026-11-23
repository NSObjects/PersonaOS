import { PlusOutlined } from '@ant-design/icons';
import {
  type ActionType, ModalForm, PageContainer, type ProColumns, ProFormSelect,
  ProFormSwitch, ProFormText, ProFormTextArea, ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { Button, message, Popconfirm, Select, Tag } from 'antd';
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
          <Select key="kind" aria-label="资源类型" style={{ width: 145 }} value={kind}
            options={kinds} onChange={setKind} />,
          ...(access.canCapabilityResourceCreate ? [
            <Button key="create" type="primary" icon={<PlusOutlined />}
              onClick={() => { setEditing(undefined); setOpen(true); }}>新增资源</Button>,
          ] : []),
        ]}
      />
      <ModalForm<CapabilityResourceInput> key={editing?.id ?? 'create'} open={open}
        title={editing ? '编辑能力资源' : '新增能力资源'}
        onOpenChange={setOpen} modalProps={{ destroyOnHidden: true }}
        initialValues={editing ?? {
          kind, code: '', name: '', reference: '', provider: '', description: '', enabled: true,
        }}
        onFinish={async (values) => {
          if (editing) {
            await updateCapabilityResource(editing.id, values);
            message.success('资源已更新');
          } else {
            await createCapabilityResource(values);
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
        <ProFormText name="provider" label="厂商（音色必填）" fieldProps={{ maxLength: 120 }} />
        <ProFormText name="reference" label="资源引用键" fieldProps={{ maxLength: 512 }}
          tooltip="仅填写已登记的引用标识，禁止存放密钥、Token、密码或脚本"
          rules={[{ required: true, message: '请输入资源引用键' }]} />
        <ProFormTextArea name="description" label="说明" fieldProps={{ rows: 3, maxLength: 2000 }} />
        <ProFormSwitch name="enabled" label="启用" />
      </ModalForm>
    </PageContainer>
  );
};

export default CapabilityResources;

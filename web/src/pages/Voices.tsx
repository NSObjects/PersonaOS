import { PlusOutlined } from '@ant-design/icons';
import { type ActionType, ModalForm, PageContainer, type ProColumns, ProFormSelect, ProFormSwitch, ProFormText, ProFormTextArea, ProTable } from '@ant-design/pro-components';
import { history, useAccess } from '@umijs/max';
import { Button, message, Popconfirm, Tag } from 'antd';
import React, { useRef, useState } from 'react';
import { type CapabilityResource, type CapabilityResourceInput, createCapabilityResource, deleteCapabilityResource, listCapabilityResources, updateCapabilityResource } from '@/services/admin';
import VoicePreview from './VoicePreview';

const cloneProvider = 'volcengine_seed3_clone';
const providers = [
  { label: '火山引擎 Seed-TTS 3.0', value: 'volcengine' },
  { label: '阿里云', value: 'aliyun' },
  { label: '腾讯云', value: 'tencent' },
];
const providerName = (provider: string) => providers.find((item) => item.value === provider)?.label ?? provider;

export default function Voices() {
  const access = useAccess();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [editing, setEditing] = useState<CapabilityResource>();
  const [open, setOpen] = useState(false);
  const [previewing, setPreviewing] = useState<CapabilityResource>();
  const isClone = editing?.provider === cloneProvider;

  const columns: ProColumns<CapabilityResource>[] = [
    { title: '音色名称', dataIndex: 'name' },
    { title: '编码', dataIndex: 'code', ellipsis: true },
    { title: '来源', render: (_, item) => <Tag>{item.provider === cloneProvider ? '声音复刻' : '手动录入'}</Tag> },
    { title: '语音厂商', render: (_, item) => providerName(item.provider === cloneProvider ? 'volcengine' : item.provider) },
    { title: '音色 ID', dataIndex: 'reference', ellipsis: true, copyable: true },
    { title: '状态', render: (_, item) => <Tag color={item.enabled ? 'success' : 'default'}>{item.enabled ? '已启用' : '已停用'}</Tag> },
    { title: '操作', valueType: 'option', render: (_, item) => [
      <Button key="preview" type="link" size="small" disabled={!item.enabled} onClick={() => setPreviewing(item)}>试听</Button>,
      ...(access.canCapabilityResourceUpdate ? [
        <Button key="edit" type="link" size="small" onClick={() => { setEditing(item); setOpen(true); }}>编辑</Button>,
      ] : []),
      ...(item.provider === cloneProvider ? [
        <Button key="clone" type="link" size="small" onClick={() => history.push('/voice-clones')}>复刻管理</Button>,
      ] : []),
      ...(access.canCapabilityResourceDelete ? [
        <Popconfirm key="delete" title="删除音色资料？" description="已被设备类型引用的音色不可删除。不会删除厂商云端音色。"
          onConfirm={async () => {
            await deleteCapabilityResource(item.id);
            message.success('音色资料已删除');
            actionRef.current?.reload();
          }}>
          <Button type="link" danger size="small">删除</Button>
        </Popconfirm>,
      ] : []),
    ] },
  ];

  return (
    <PageContainer title="音色管理" subTitle="统一管理官方音色和克隆音色，设备类型只引用此处的音色资料。">
      <ProTable<CapabilityResource> rowKey="id" actionRef={actionRef} columns={columns} search={false} pagination={false}
        request={async () => {
          const data = await listCapabilityResources('voice');
          return { data, total: data.length, success: true };
        }}
        toolBarRender={() => [
          <Button key="clone" onClick={() => history.push('/voice-clones')}>声音复刻</Button>,
          ...(access.canCapabilityResourceCreate ? [
            <Button key="create" type="primary" icon={<PlusOutlined />} onClick={() => { setEditing(undefined); setOpen(true); }}>新增音色</Button>,
          ] : []),
        ]}
      />
      {previewing && <VoicePreview key={previewing.id} resource={previewing} onClose={() => setPreviewing(undefined)} />}
      {open && <ModalForm<CapabilityResourceInput> key={editing?.id ?? 'new'} open
        title={editing ? '编辑音色资料' : '新增音色'} modalProps={{ destroyOnHidden: true }}
        onOpenChange={setOpen} initialValues={editing ?? { kind: 'voice', provider: 'volcengine', enabled: true }}
        onFinish={async (values) => {
          const input: CapabilityResourceInput = {
            ...values, kind: 'voice',
            code: editing?.code ?? values.code,
            provider: isClone ? editing.provider : values.provider,
            reference: isClone ? editing.reference : values.reference,
            enabled: isClone ? editing.enabled : values.enabled,
          };
          if (editing) await updateCapabilityResource(editing.id, input);
          else await createCapabilityResource(input);
          message.success('音色资料已保存');
          actionRef.current?.reload();
          return true;
        }}>
        <ProFormText name="code" label="唯一编码" disabled={Boolean(editing)}
          rules={[{ required: true }, { pattern: /^[a-zA-Z][a-zA-Z0-9_-]{1,63}$/, message: '2–64 位，英文开头，仅支持字母、数字、下划线、连字符' }]} />
        <ProFormText name="name" label="音色名称" rules={[{ required: true }]} fieldProps={{ maxLength: 50 }} />
        <ProFormSelect name="provider" label="语音厂商" disabled={isClone}
          options={isClone ? [{ label: '火山引擎 Seed-TTS 3.0（复刻）', value: cloneProvider }] : providers}
          rules={[{ required: true }]} />
        <ProFormText name="reference" label="音色 ID" disabled={isClone} rules={[{ required: true }]}
          tooltip="填写厂商官方音色 ID 或已取得的合法克隆音色 ID。火山引擎仅使用 3.0 音色。" />
        <ProFormTextArea name="description" label="说明" fieldProps={{ rows: 3, maxLength: 600 }} />
        <ProFormSwitch name="enabled" label="启用" disabled={isClone} />
        {isClone && <Button type="link" onClick={() => history.push('/voice-clones')}>在声音复刻页面管理训练与启用状态</Button>}
      </ModalForm>}
    </PageContainer>
  );
}

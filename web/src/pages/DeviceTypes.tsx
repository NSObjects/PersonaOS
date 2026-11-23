import { PlusOutlined } from '@ant-design/icons';
import {
  type ActionType, ModalForm, PageContainer, type ProColumns, ProFormSelect,
  ProFormSwitch, ProFormText, ProFormTextArea, ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { Button, message, Popconfirm } from 'antd';
import React, { useEffect, useRef, useState } from 'react';
import {
  type Character, type DeviceType, type DeviceTypeInput, createDeviceType,
  deleteDeviceType, listCharacters, listDeviceTypes, updateDeviceType,
} from '@/services/admin';

const DeviceTypes: React.FC = () => {
  const access = useAccess();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [characters, setCharacters] = useState<Character[]>([]);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<DeviceType>();

  useEffect(() => { void listCharacters().then(setCharacters).catch(() => setCharacters([])); }, []);
  const characterName = (id: number) => characters.find((item) => item.id === id)?.name ?? ('#' + id);

  const columns: ProColumns<DeviceType>[] = [
    { title: '设备型号', dataIndex: 'code' },
    { title: '名称', dataIndex: 'name' },
    { title: '角色', dataIndex: 'character_id', render: (_, item) => characterName(item.character_id) },
    { title: '摄像头', dataIndex: 'has_camera', search: false, render: (_, item) => item.has_camera ? '支持' : '无' },
    { title: '麦克风', dataIndex: 'has_microphone', search: false, render: (_, item) => item.has_microphone ? '支持' : '无' },
    { title: '扬声器', dataIndex: 'has_speaker', search: false, render: (_, item) => item.has_speaker ? '支持' : '无' },
    {
      title: '操作', valueType: 'option',
      render: (_, item) => [
        ...(access.canDeviceTypeUpdate ? [<a key="edit" onClick={() => { setEditing(item); setOpen(true); }}>编辑</a>] : []),
        ...(access.canDeviceTypeDelete ? [
          <Popconfirm key="delete" title="删除设备类型" description={'确认删除 ' + item.code + '？不会删除关联角色。'}
            onConfirm={async () => { await deleteDeviceType(item.id); message.success('设备类型已删除'); actionRef.current?.reload(); }}>
            <Button type="link" danger size="small">删除</Button>
          </Popconfirm>,
        ] : []),
      ],
    },
  ];

  return (
    <PageContainer title="设备类型">
      <ProTable<DeviceType> rowKey="id" actionRef={actionRef} columns={columns}
        search={false} pagination={false}
        request={async () => { const data = await listDeviceTypes(); return { data, success: true, total: data.length }; }}
        toolBarRender={() => access.canDeviceTypeCreate ? [
          <Button key="create" type="primary" icon={<PlusOutlined />} onClick={() => { setEditing(undefined); setOpen(true); }}>
            新增设备类型
          </Button>,
        ] : []}
      />
      <ModalForm<DeviceTypeInput> key={editing?.id ?? 'create'} title={editing ? '编辑设备类型' : '新增设备类型'}
        open={open} onOpenChange={setOpen} modalProps={{ destroyOnHidden: true }}
        initialValues={editing ?? {
          code: '', name: '', character_id: undefined,
          has_camera: false, has_microphone: true, has_speaker: true,
        }}
        onFinish={async (values) => {
          if (editing) { await updateDeviceType(editing.id, values); message.success('设备类型已更新'); }
          else { await createDeviceType(values); message.success('设备类型已创建'); }
          actionRef.current?.reload();
          return true;
        }}>
        <ProFormText name="code" label="型号编码" disabled={Boolean(editing)}
          rules={[{ required: !editing, message: '请输入型号编码' },
            { pattern: /^[a-zA-Z0-9][a-zA-Z0-9_-]{1,63}$/, message: '请输入 2–64 位英文、数字、下划线或连字符' }]} />
        <ProFormText name="name" label="型号名称" fieldProps={{ maxLength: 160 }}
          rules={[{ required: true, message: '请输入型号名称' }]} />
        <ProFormSelect name="character_id" label="关联角色"
          options={characters.map((item) => ({ label: item.name, value: item.id }))}
          rules={[{ required: true, message: '请选择角色' }]} />
        <ProFormTextArea name="description" label="说明" fieldProps={{ rows: 2, maxLength: 2000 }} />
        <ProFormSwitch name="has_camera" label="摄像头" />
        <ProFormSwitch name="has_microphone" label="麦克风" />
        <ProFormSwitch name="has_speaker" label="扬声器" />
      </ModalForm>
    </PageContainer>
  );
};

export default DeviceTypes;

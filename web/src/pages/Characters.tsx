import { PlusOutlined } from '@ant-design/icons';
import { type ActionType, ModalForm, PageContainer, type ProColumns, ProFormText, ProFormTextArea, ProTable } from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { Button, message, Popconfirm } from 'antd';
import React, { useRef, useState } from 'react';
import { type Character, type CharacterInput, createCharacter, deleteCharacter, listCharacters, updateCharacter } from '@/services/admin';

const Characters: React.FC = () => {
  const access = useAccess();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Character>();

  const columns: ProColumns<Character>[] = [
    { title: '角色名称', dataIndex: 'name' },
    { title: '编码', dataIndex: 'code' },
    { title: '人格设定', dataIndex: 'persona', ellipsis: true, search: false },
    { title: '说明', dataIndex: 'description', ellipsis: true, search: false },
    {
      title: '操作', valueType: 'option',
      render: (_, item) => [
        ...(access.canCharacterUpdate ? [
          <a key="edit" onClick={() => { setEditing(item); setOpen(true); }}>编辑</a>,
        ] : []),
        ...(access.canCharacterDelete ? [
          <Popconfirm key="delete" title="删除角色"
            description={'确认删除 ' + item.name + '？已被设备类型引用的角色不可删除。'}
            onConfirm={async () => {
              await deleteCharacter(item.id);
              message.success('角色已删除');
              actionRef.current?.reload();
            }}>
            <Button type="link" danger size="small">删除</Button>
          </Popconfirm>,
        ] : []),
      ],
    },
  ];

  return (
    <PageContainer title="角色资源">
      <ProTable<Character> rowKey="id" actionRef={actionRef} columns={columns}
        search={false} pagination={false}
        request={async () => {
          const data = await listCharacters();
          return { data, success: true, total: data.length };
        }}
        toolBarRender={() => access.canCharacterCreate ? [
          <Button key="create" type="primary" icon={<PlusOutlined />}
            onClick={() => { setEditing(undefined); setOpen(true); }}>新增角色</Button>,
        ] : []}
      />
      <ModalForm<CharacterInput> key={editing?.id ?? 'create'} title={editing ? '编辑角色' : '新增角色'}
        open={open} onOpenChange={setOpen} modalProps={{ destroyOnHidden: true }}
        initialValues={editing ?? { code: '', name: '', persona: '', description: '' }}
        onFinish={async (values) => {
          if (editing) {
            await updateCharacter(editing.id, values);
            message.success('角色已更新');
          } else {
            await createCharacter(values);
            message.success('角色已创建');
          }
          actionRef.current?.reload();
          return true;
        }}>
        <ProFormText name="code" label="角色编码" disabled={Boolean(editing)}
          rules={[{ required: !editing, message: '请输入角色编码' },
            { pattern: /^[a-zA-Z0-9][a-zA-Z0-9_-]{1,63}$/, message: '请输入 2–64 位英文、数字、下划线或连字符' }]} />
        <ProFormText name="name" label="名称" fieldProps={{ maxLength: 160 }}
          rules={[{ required: true, message: '请输入名称' }]} />
        <ProFormTextArea name="persona" label="人格设定"
          fieldProps={{ rows: 6, maxLength: 16000 }}
          rules={[{ required: true, message: '请输入人格设定' }]} />
        <ProFormTextArea name="description" label="说明" fieldProps={{ rows: 2, maxLength: 2000 }} />
      </ModalForm>
    </PageContainer>
  );
};

export default Characters;

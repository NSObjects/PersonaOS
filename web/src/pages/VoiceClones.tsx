import { PlusOutlined } from '@ant-design/icons';
import { type ActionType, PageContainer, type ProColumns, ProTable } from '@ant-design/pro-components';
import { history, useAccess } from '@umijs/max';
import { Button, Tag } from 'antd';
import React, { useRef, useState } from 'react';
import { type CapabilityResource, listCapabilityResources } from '@/services/admin';
import VoiceClonePanel from './VoiceClonePanel';

export default function VoiceClones() {
  const access = useAccess();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<CapabilityResource>();
  const show = (resource?: CapabilityResource) => { setSelected(resource); setOpen(true); };
  const columns: ProColumns<CapabilityResource>[] = [
    { title: '音色名称', dataIndex: 'name' },
    { title: '编码', dataIndex: 'code', ellipsis: true },
    { title: '音色 ID', dataIndex: 'reference', ellipsis: true, copyable: true },
    { title: '资料状态', render: (_, item) => <Tag color={item.enabled ? 'success' : 'default'}>{item.enabled ? '已启用' : '待确认启用'}</Tag> },
    { title: '操作', valueType: 'option', render: (_, item) => [
      <Button key="manage" type="link" size="small" onClick={() => show(item)}>训练与试听</Button>,
    ] },
  ];

  return (
    <PageContainer title="声音复刻" subTitle="上传授权录音、训练和试听。确认启用后，同一份音色资料即可用于设备绑定。">
      <ProTable<CapabilityResource> rowKey="id" columns={columns} actionRef={actionRef} search={false} pagination={false}
        request={async () => {
          const data = (await listCapabilityResources('voice')).filter((item) => item.provider === 'volcengine_seed3_clone');
          return { data, total: data.length, success: true };
        }}
        toolBarRender={() => [
          <Button key="catalog" onClick={() => history.push('/voices')}>音色管理</Button>,
          ...(access.canCapabilityResourceCreate ? [
            <Button key="create" type="primary" icon={<PlusOutlined />} onClick={() => show()}>新建克隆音色</Button>,
          ] : []),
        ]}
      />
      {open && <VoiceClonePanel key={selected?.id ?? 'new'} initial={selected} canTrain={access.canCapabilityResourceUpdate}
        onClose={() => setOpen(false)} onChanged={() => actionRef.current?.reload()} />}
    </PageContainer>
  );
}

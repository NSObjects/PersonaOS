import { Button, Descriptions, message, Modal, Popconfirm, Space, Tag } from 'antd';
import React, { useState } from 'react';

import {
  type ActiveDeviceConfig, type CapabilityResource, type DeviceType,
  applyDeviceTypeConfig, getActiveDeviceConfig,
} from '@/services/admin';

type Props = {
  device: DeviceType;
  canApply: boolean;
};

function names(items: CapabilityResource[]): string {
  return items.map((item) => item.name).join('、') || '无';
}

// ActiveConfigActions is intentionally Demo-only: one confirmed snapshot
// per dynamically configured device type, without history or rollback.
const ActiveConfigActions: React.FC<Props> = ({ device, canApply }) => {
  const [active, setActive] = useState<ActiveDeviceConfig>();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  const showActive = async () => {
    setBusy(true);
    try {
      const config = await getActiveDeviceConfig(device.id);
      setActive(config);
      setOpen(true);
    } catch {
      message.warning('尚无可查看的生效配置，请先应用草稿');
    } finally {
      setBusy(false);
    }
  };

  const apply = async () => {
    setBusy(true);
    try {
      const config = await applyDeviceTypeConfig(device.id);
      setActive(config);
      setOpen(true);
      message.success('配置已应用到 Demo');
    } catch {
      message.error('配置应用失败，请检查资源是否已停用');
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Button type="link" size="small" loading={busy}
        onClick={() => { void showActive(); }}>生效配置</Button>
      {canApply && (
        <Popconfirm title="应用当前草稿到 Demo？"
          description="仅更新此设备类型的生效配置，不影响其他型号，也不直接执行 Tool。"
          onConfirm={apply}>
          <Button size="small" type="link" disabled={busy}>应用配置</Button>
        </Popconfirm>
      )}
      <Modal title={`生效配置 · ${device.code}`} open={open}
        onCancel={() => setOpen(false)} footer={null} destroyOnHidden width={620}>
        {active && (
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label="设备类型">{active.device_type.name}</Descriptions.Item>
            <Descriptions.Item label="角色">{active.character.name}</Descriptions.Item>
            <Descriptions.Item label="人格">{active.character.persona}</Descriptions.Item>
            <Descriptions.Item label="音色">{active.voice?.name ?? '角色默认 / 未覆盖'}</Descriptions.Item>
            <Descriptions.Item label="Tools">{names(active.tools)}</Descriptions.Item>
            <Descriptions.Item label="Skills">{names(active.skills)}</Descriptions.Item>
            <Descriptions.Item label="知识库">{names(active.knowledge_bases)}</Descriptions.Item>
            <Descriptions.Item label="配置状态">
              <Space size="small"><Tag color="success">已生效</Tag>第 {active.version} 次应用</Space>
            </Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </>
  );
};

export default ActiveConfigActions;

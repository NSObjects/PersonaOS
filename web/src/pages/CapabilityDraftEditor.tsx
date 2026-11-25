import { Alert, Form, message, Modal, Select, Spin } from 'antd';
import React, { useEffect, useState } from 'react';

import {
  type CapabilityResource, type CapabilityResourceKind,
  type DeviceType, type DeviceTypeCapabilityDraftInput,
  getDeviceTypeCapabilityDraft, listCapabilityResources, saveDeviceTypeCapabilityDraft,
} from '@/services/admin';

const kinds: CapabilityResourceKind[] = ['voice', 'tool', 'skill', 'knowledge_base'];

type ResourceCatalog = Record<CapabilityResourceKind, CapabilityResource[]>;

const emptyCatalog = (): ResourceCatalog => ({
  voice: [], tool: [], skill: [], knowledge_base: [],
});

type Props = {
  device: DeviceType;
  canEdit: boolean;
  onClose: () => void;
};

function resourceOptions(resources: CapabilityResource[]) {
  return resources.map((resource) => ({
    value: resource.id,
    label: `${resource.name} (${resource.code})${resource.enabled ? '' : ' — 已停用'}`,
  }));
}

// CapabilityDraftEditor only edits unpublished references for one device model.
const CapabilityDraftEditor: React.FC<Props> = ({ device, canEdit, onClose }) => {
  const [form] = Form.useForm<DeviceTypeCapabilityDraftInput>();
  const [resources, setResources] = useState<ResourceCatalog>(emptyCatalog);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setLoadError(false);

    void Promise.all([
      getDeviceTypeCapabilityDraft(device.id),
      Promise.all(kinds.map((kind) => listCapabilityResources(kind))),
    ]).then(([draft, lists]) => {
      if (!active) return;
      form.setFieldsValue({
        voice_id: draft.voice_id || undefined,
        tool_ids: draft.tool_ids ?? [],
        skill_ids: draft.skill_ids ?? [],
        knowledge_base_ids: draft.knowledge_base_ids ?? [],
      });
      const catalog = emptyCatalog();
      kinds.forEach((kind, index) => { catalog[kind] = lists[index]; });
      setResources(catalog);
    }).catch(() => {
      if (active) setLoadError(true);
    }).finally(() => {
      if (active) setLoading(false);
    });

    return () => { active = false; };
  }, [device.id, form]);

  const save = async () => {
    if (!canEdit || loading || loadError || saving) return;
    const values = await form.validateFields();
    setSaving(true);
    try {
      await saveDeviceTypeCapabilityDraft(device.id, {
        voice_id: values.voice_id ?? 0,
        tool_ids: values.tool_ids ?? [],
        skill_ids: values.skill_ids ?? [],
        knowledge_base_ids: values.knowledge_base_ids ?? [],
      });
      message.success('能力草稿已保存（尚未发布）');
      onClose();
    } catch {
      message.error('草稿保存失败，请检查资源是否仍处于启用状态');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal title={`能力草稿 · ${device.code}`} open onCancel={onClose}
      onOk={() => { void save(); }} okText="保存草稿" okButtonProps={{
        disabled: !canEdit || loading || loadError, loading: saving,
      }} width={620} destroyOnHidden>
      <Alert type="info" showIcon style={{ marginBottom: 16 }}
        message="仅修改此设备类型的草稿，不会发布到设备，也不会授予 Agent 工具执行权限。" />
      {loadError && <Alert type="error" showIcon style={{ marginBottom: 16 }}
        message="草稿或能力资源加载失败，请确认当前角色已获得相关受管 API 授权，然后重新打开。" />}
      <Spin spinning={loading}>
        <Form<DeviceTypeCapabilityDraftInput> form={form} layout="vertical"
          disabled={!canEdit || loading || loadError}>
          <Form.Item name="voice_id" label="语音音色"
            extra="选择音色管理中的一条资料；不需要填写语音厂商或音色 ID。留空表示无音色覆盖。">
            <Select allowClear placeholder="选择音色" options={resourceOptions(resources.voice)} />
          </Form.Item>
          <Form.Item name="tool_ids" label="Tools"
            extra="可绑定多个 Tool，摄像头硬件能力与视觉 Tool 授权是不同概念。">
            <Select mode="multiple" allowClear placeholder="选择 Tools"
              options={resourceOptions(resources.tool)} />
          </Form.Item>
          <Form.Item name="skill_ids" label="Skills">
            <Select mode="multiple" allowClear placeholder="选择 Skills"
              options={resourceOptions(resources.skill)} />
          </Form.Item>
          <Form.Item name="knowledge_base_ids" label="知识库">
            <Select mode="multiple" allowClear placeholder="选择知识库"
              options={resourceOptions(resources.knowledge_base)} />
          </Form.Item>
        </Form>
      </Spin>
      <Alert type="warning" showIcon
        message="标记为「已停用」的资源只能从草稿中移除；继续保留这些引用会被服务端拒绝。" />
    </Modal>
  );
};

export default CapabilityDraftEditor;

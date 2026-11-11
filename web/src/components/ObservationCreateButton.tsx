import { useState } from 'react';
import { Alert, Button, Modal, Select, Space, Typography, message } from 'antd';
import { Link } from 'react-router-dom';
import { api, type MethodCard, type SelectionCandidate } from '../api/client';

interface Props {
  runID: string;
  candidate: SelectionCandidate;
  defaultMethodID?: string;
  methodCards?: Record<string, MethodCard>;
}

export default function ObservationCreateButton({ runID, candidate, defaultMethodID, methodCards }: Props) {
  const [open, setOpen] = useState(false);
  const [methodID, setMethodID] = useState(defaultMethodID || candidate.triggers[0]?.method_id);
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<Record<string, string>>({});
  const eligible = candidate.triggers.filter((trigger) => trigger.method_version_id);
  const selectedCard = methodCards?.[methodID || ''];
  const targetMissing = !!selectedCard && selectedCard.outcome?.target_return_pct == null;
  const observationID = created[methodID || ''];

  const submit = async () => {
    if (!runID || !methodID || observationID) return;
    setBusy(true);
    try {
      const result = await api.observationCreate(runID, candidate.code, methodID);
      setCreated((prev) => ({ ...prev, [methodID]: result.contract.id }));
      void message.success('已冻结当时方法和目标；后台将更新本地走势验证');
    } catch (error) {
      void message.error(error instanceof Error ? error.message : '加入走势验证失败');
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Button size="small" disabled={!runID || !eligible.length || targetMissing} onClick={() => setOpen(true)}>
        加入走势验证
      </Button>
      <Modal
        cancelText="取消"
        title={`${candidate.code} · 持续验证当时的方法`}
        open={open}
        onCancel={() => setOpen(false)}
        confirmLoading={busy}
        onOk={() => void submit()}
        okText={observationID ? '已加入' : '确认加入'}
        okButtonProps={{ disabled: !!observationID || !runID || !methodID || targetMissing }}
      >
        <Space orientation="vertical" style={{ display: 'flex' }} size={12}>
          <Typography.Text>选择一个触发方法；其他方法可分别加入，不合并目标或胜率。</Typography.Text>
          <Select
            aria-label="选择验证方法"
            value={methodID}
            onChange={setMethodID}
            options={eligible.map((trigger) => ({
              value: trigger.method_id,
              label: `${trigger.method_name}${created[trigger.method_id] ? '（已加入）' : ''}`,
            }))}
            style={{ width: '100%' }}
          />
          {targetMissing && <Alert type="warning" showIcon title="该方法版本没有明确观察目标，不能加入走势验证" />}
          <Alert
            type="info"
            showIcon
            title="以选股当天冻结的收盘价为观察基准"
            description="目标、期限、方法版本由服务端从该次选股记录冻结。只验证未来走势，不下单、不模拟成交；方法没定义目标时会如实显示。"
          />
          {observationID && (
            <Link to={`/observations?id=${encodeURIComponent(observationID)}`}>查看这条走势验证</Link>
          )}
        </Space>
      </Modal>
    </>
  );
}

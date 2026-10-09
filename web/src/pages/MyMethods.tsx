import { useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Empty,
  Input,
  Popconfirm,
  Select,
  Skeleton,
  Space,
  Tag,
  Typography,
} from 'antd';
import {
  ExperimentOutlined,
  PlusOutlined,
  ReloadOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import MethodSummaryCard from '../components/methods/MethodSummaryCard';
import MethodValidationDrawer from '../components/methods/MethodValidationDrawer';
import { useMethodCatalog } from '../components/methods/useMethodCatalog';
import { useMethodDiscovery } from '../components/methods/useMethodDiscovery';
import { methodCanScreen } from '../lib/methodPresentation';
import '../components/methods/methods.css';

const { Paragraph, Text, Title } = Typography;
type AvailabilityFilter = 'all' | 'ready' | 'pending';

export default function MyMethods() {
  const [query, setQuery] = useState('');
  const [availability, setAvailability] = useState<AvailabilityFilter>('all');
  const {
    items, loading, error, screeningId, seeding,
    drawerOpen, setDrawerOpen, detail, audit, detailLoading, detailError,
    load, screenMethod, loadValidation, seedMethods,
  } = useMethodCatalog();
  const {
    status: researchStatus,
    result: researchResult,
    starting: researchStarting,
    error: researchError,
    start: runResearch,
  } = useMethodDiscovery(load);

  const visible = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return items
      .filter((method) => {
        if (availability === 'ready' && !methodCanScreen(method)) return false;
        if (availability === 'pending' && methodCanScreen(method)) return false;
        if (!normalized) return true;
        return [method.name, method.entry_summary, method.market, method.universe]
          .some((value) => value?.toLowerCase().includes(normalized));
      })
      .sort((a, b) => {
        const eligibility = Number(methodCanScreen(b)) - Number(methodCanScreen(a));
        if (eligibility !== 0) return eligibility;
        return (b.evidence?.oos_win_rate ?? -1) - (a.evidence?.oos_win_rate ?? -1);
      });
  }, [availability, items, query]);

  return (
    <Space className="methods-page" orientation="vertical" size={20}>
      <header className="methods-page__intro">
        <div>
          <Title level={2}>我的选股方法</Title>
          <Paragraph type="secondary" style={{ margin: 0, maxWidth: 760 }}>
            先看一句话规则和历史样本外结果，再用通过验证的方法筛选今日股票。历史验证胜率是统计结果，不是未来上涨概率。
          </Paragraph>
        </div>
        <Space wrap>
          <Popconfirm
            title="帮我挖新方法"
            description="系统会在冻结历史数据上验证候选规则；接入 AI/provider 时由它提出每个方法自己的窗口、目标和股票池。未接入时只运行明确标记的确定性兼容扫描，不会冒充 AI 结论。一轮可能耗时数十分钟。"
            okText="开始挖掘"
            cancelText="取消"
            onConfirm={() => void runResearch()}
          >
            <Button
              type="primary"
              icon={<PlusOutlined />}
              loading={researchStarting}
              disabled={researchStatus?.running}
            >
              {researchStatus?.running ? '正在挖掘新方法' : '帮我挖新方法'}
            </Button>
          </Popconfirm>
          <Button href="/methods/advanced" icon={<SettingOutlined />}>高级研究与管理</Button>
        </Space>
      </header>

      {researchStatus?.running && (
        <Alert
          type="info"
          showIcon
          title="正在挖掘和验证新方法"
          description={(() => {
            const progress = researchStatus.progress;
            const phase = researchStatus.phase === 'discovery' ? '发现候选规则' : researchStatus.phase === 'validation' ? '样本外验证' : '准备数据';
            const scan = progress?.discovery_codes_total ? `扫描 ${progress.discovery_codes_done ?? 0}/${progress.discovery_codes_total} 只股票` : '';
            const candidates = progress?.total_candidates ? `候选 ${progress.candidates_done ?? 0}/${progress.total_candidates}` : '';
            const outcomes = `已通过 ${progress?.verified ?? 0} · 未通过 ${progress?.rejected ?? 0}`;
            return `当前阶段：${phase}${scan ? `，${scan}` : ''}${candidates ? `，${candidates}` : ''}，${outcomes}。页面可以离开，完成后方法列表会更新。`;
          })()}
        />
      )}
      {!researchStatus?.running && researchResult && (
        <Alert
          type={researchResult.verified > 0 ? 'success' : 'warning'}
          showIcon
          title={researchResult.verified > 0 ? `最近一轮挖掘收下 ${researchResult.verified} 个新方法` : '最近一轮没有新方法通过验证'}
          description={`候选中 ${researchResult.rejected} 个未达到证据门槛；这些方法不会进入今日选股。`}
        />
      )}
      {researchError && <Alert type="error" showIcon title="新方法挖掘启动失败" description={researchError} />}
      {error && <Alert type="error" showIcon title="方法列表读取失败" description={error} action={<Button size="small" onClick={() => void load()}>重试</Button>} />}

      <div className="methods-page__toolbar">
        <Space wrap>
          <Input.Search
            allowClear
            aria-label="搜索选股方法"
            placeholder="搜索规则、方法名或股票池"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            style={{ width: 300, maxWidth: '100%' }}
          />
          <Select<AvailabilityFilter>
            aria-label="按可用状态筛选"
            value={availability}
            onChange={setAvailability}
            options={[
              { value: 'all', label: '全部方法' },
              { value: 'ready', label: '可用于今日筛选' },
              { value: 'pending', label: '待补充验证' },
            ]}
            style={{ width: 190 }}
          />
        </Space>
        <Space>
          <Tag>{visible.length} 个方法</Tag>
          <Button aria-label="刷新方法列表" icon={<ReloadOutlined />} onClick={() => void load()} />
        </Space>
      </div>

      {loading ? (
        <Space orientation="vertical" size={12} style={{ display: 'flex' }} aria-busy="true" aria-label="正在读取选股方法">
          {[0, 1, 2].map((key) => <Skeleton key={key} active paragraph={{ rows: 3 }} />)}
        </Space>
      ) : visible.length ? (
        <Space orientation="vertical" size={12} style={{ display: 'flex' }}>
          {visible.map((method) => (
            <MethodSummaryCard
              key={method.id}
              method={method}
              screening={screeningId === method.id}
              onScreen={(target) => void screenMethod(target)}
              onValidation={(target) => void loadValidation(target)}
            />
          ))}
        </Space>
      ) : (
        <Empty
          description={query || availability !== 'all' ? '没有符合当前条件的方法' : '还没有可展示的选股方法'}
        >
          <Space wrap>
            {(query || availability !== 'all') ? (
              <Button onClick={() => { setQuery(''); setAvailability('all'); }}>清空筛选</Button>
            ) : (
              <>
                <Button type="primary" icon={<ExperimentOutlined />} loading={seeding} onClick={() => void seedMethods()}>验证入门方法</Button>
                <Button icon={<PlusOutlined />} onClick={() => void runResearch()}>帮我挖新方法</Button>
              </>
            )}
          </Space>
        </Empty>
      )}

      <Text type="secondary">方法只有在历史样本外验证和证据等级同时达标后，才能用于“筛选今日股票”。</Text>

      <MethodValidationDrawer
        open={drawerOpen}
        method={detail}
        audit={audit}
        loading={detailLoading}
        error={detailError}
        onClose={() => setDrawerOpen(false)}
        onRetry={() => detail && void loadValidation(detail)}
      />
    </Space>
  );
}

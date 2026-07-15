import { useEffect, useMemo, useState } from 'react';
import {
  DeleteOutlined,
  ExclamationCircleOutlined,
  InfoCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Flex,
  Input,
  InputNumber,
  Modal,
  Skeleton,
  Space,
  Tabs,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import { api } from '../../api/client';
import type { IndicatorConfig, IndicatorParams } from '../../types/api';

const { Paragraph, Text, Title } = Typography;

const DEFAULT_PARAMS: IndicatorParams = {
  ma: [5, 10, 20, 60, 120],
  macd: { fast: 12, slow: 26, signal: 9 },
  kdj: { n: 9, m1: 3, m2: 3 },
  boll: { n: 20, k: 2.0 },
  rsi: [6, 12, 24],
};

const INITIAL_CONFIG: IndicatorConfig = {
  defaults: { ...DEFAULT_PARAMS, ma: [...DEFAULT_PARAMS.ma], rsi: [...DEFAULT_PARAMS.rsi] },
  categories: {
    large_cap: { ma: [5, 10, 20, 60, 120], macd: { fast: 12, slow: 26, signal: 9 } },
    small_cap: { ma: [5, 10, 20], macd: { fast: 8, slow: 17, signal: 9 }, kdj: { n: 7, m1: 3, m2: 3 } },
  },
  overrides: {
    '000001': { kdj: { n: 5, m1: 3, m2: 3 } },
  },
};

const CATEGORY_LABELS: Record<string, string> = {
  large_cap: '大盘股',
  small_cap: '小盘股',
};

const PARAM_OPTIONS: { key: keyof IndicatorParams; label: string }[] = [
  { key: 'ma', label: 'MA 均线' },
  { key: 'macd', label: 'MACD' },
  { key: 'kdj', label: 'KDJ' },
  { key: 'boll', label: 'BOLL' },
  { key: 'rsi', label: 'RSI' },
];

function cloneIndicatorConfig(config: IndicatorConfig): IndicatorConfig {
  return structuredClone(config);
}

function NumberTagEditor({
  label,
  values,
  onChange,
  placeholder,
}: {
  label: string;
  values: number[];
  onChange: (values: number[]) => void;
  placeholder: string;
}) {
  const [input, setInput] = useState<number | null>(null);

  const addValue = () => {
    if (!input || input <= 0 || values.includes(input)) return;
    onChange([...values, input].sort((a, b) => a - b));
    setInput(null);
  };

  return (
    <Card size="small" title={label}>
      <Space wrap size={[8, 8]}>
        {values.map((value) => (
          <Tag
            key={value}
            closable
            onClose={() => onChange(values.filter((item) => item !== value))}
            color="blue"
            style={{ paddingInline: 10, paddingBlock: 4 }}
          >
            {value}
          </Tag>
        ))}
        <InputNumber
          min={1}
          value={input ?? undefined}
          onChange={(value) => setInput(typeof value === 'number' ? value : null)}
          onPressEnter={addValue}
          placeholder={placeholder}
          style={{ width: 110 }}
        />
        <Button icon={<PlusOutlined />} onClick={addValue}>添加</Button>
      </Space>
    </Card>
  );
}

function ParamGroupCard({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <Card size="small" title={title} extra={description ? <Text type="secondary">{description}</Text> : null}>
      {children}
    </Card>
  );
}

function IndicatorEditors({
  params,
  onChange,
}: {
  params: IndicatorParams;
  onChange: (next: IndicatorParams) => void;
}) {
  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 16 }}>
        <NumberTagEditor
          label="MA 均线"
          values={params.ma}
          onChange={(ma) => onChange({ ...params, ma })}
          placeholder="周期"
        />
        <NumberTagEditor
          label="RSI 周期"
          values={params.rsi}
          onChange={(rsi) => onChange({ ...params, rsi })}
          placeholder="周期"
        />
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 16 }}>
        <ParamGroupCard title="MACD" description="快线 / 慢线 / 信号线">
          <Space>
            <InputNumber min={1} value={params.macd.fast} onChange={(value) => value && onChange({ ...params, macd: { ...params.macd, fast: value } })} addonBefore="Fast" />
            <InputNumber min={1} value={params.macd.slow} onChange={(value) => value && onChange({ ...params, macd: { ...params.macd, slow: value } })} addonBefore="Slow" />
            <InputNumber min={1} value={params.macd.signal} onChange={(value) => value && onChange({ ...params, macd: { ...params.macd, signal: value } })} addonBefore="Signal" />
          </Space>
        </ParamGroupCard>

        <ParamGroupCard title="KDJ" description="N / M1 / M2">
          <Space>
            <InputNumber min={1} value={params.kdj.n} onChange={(value) => value && onChange({ ...params, kdj: { ...params.kdj, n: value } })} addonBefore="N" />
            <InputNumber min={1} value={params.kdj.m1} onChange={(value) => value && onChange({ ...params, kdj: { ...params.kdj, m1: value } })} addonBefore="M1" />
            <InputNumber min={1} value={params.kdj.m2} onChange={(value) => value && onChange({ ...params, kdj: { ...params.kdj, m2: value } })} addonBefore="M2" />
          </Space>
        </ParamGroupCard>

        <ParamGroupCard title="BOLL" description="周期 / 标准差倍数">
          <Space>
            <InputNumber min={1} value={params.boll.n} onChange={(value) => value && onChange({ ...params, boll: { ...params.boll, n: value } })} addonBefore="N" />
            <InputNumber min={0.1} step={0.1} value={params.boll.k} onChange={(value) => value && onChange({ ...params, boll: { ...params.boll, k: value } })} addonBefore="K" />
          </Space>
        </ParamGroupCard>
      </div>
    </Space>
  );
}

function PartialParamsEditor({
  params,
  defaults,
  onChange,
}: {
  params: Partial<IndicatorParams>;
  defaults: IndicatorParams;
  onChange: (next: Partial<IndicatorParams>) => void;
}) {
  const [newPeriod, setNewPeriod] = useState<Record<string, number | null>>({});
  const inactiveKeys = PARAM_OPTIONS.filter((option) => !(option.key in params));

  const update = <K extends keyof IndicatorParams>(key: K, value: IndicatorParams[K]) => {
    onChange({ ...params, [key]: value });
  };

  const remove = (key: keyof IndicatorParams) => {
    const next = { ...params };
    delete next[key];
    onChange(next);
  };

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      {params.ma && (
        <ParamGroupCard title="MA 均线覆盖">
          <Space wrap>
            {params.ma.map((value) => (
              <Tag key={value} closable onClose={() => update('ma', params.ma!.filter((item) => item !== value))} color="blue">{value}</Tag>
            ))}
            <InputNumber
              min={1}
              value={newPeriod.ma ?? undefined}
              onChange={(value) => setNewPeriod((prev) => ({ ...prev, ma: typeof value === 'number' ? value : null }))}
              placeholder="新周期"
            />
            <Button
              icon={<PlusOutlined />}
              onClick={() => {
                const value = newPeriod.ma;
                if (!value || params.ma!.includes(value)) return;
                update('ma', [...params.ma!, value].sort((a, b) => a - b));
                setNewPeriod((prev) => ({ ...prev, ma: null }));
              }}
            >
              添加
            </Button>
            <Button danger type="text" onClick={() => remove('ma')}>移除配置</Button>
          </Space>
        </ParamGroupCard>
      )}

      {params.macd && (
        <ParamGroupCard title="MACD 覆盖">
          <Space>
            <InputNumber min={1} value={params.macd.fast} onChange={(value) => value && update('macd', { ...params.macd!, fast: value })} addonBefore="Fast" />
            <InputNumber min={1} value={params.macd.slow} onChange={(value) => value && update('macd', { ...params.macd!, slow: value })} addonBefore="Slow" />
            <InputNumber min={1} value={params.macd.signal} onChange={(value) => value && update('macd', { ...params.macd!, signal: value })} addonBefore="Signal" />
            <Button danger type="text" onClick={() => remove('macd')}>移除配置</Button>
          </Space>
        </ParamGroupCard>
      )}

      {params.kdj && (
        <ParamGroupCard title="KDJ 覆盖">
          <Space>
            <InputNumber min={1} value={params.kdj.n} onChange={(value) => value && update('kdj', { ...params.kdj!, n: value })} addonBefore="N" />
            <InputNumber min={1} value={params.kdj.m1} onChange={(value) => value && update('kdj', { ...params.kdj!, m1: value })} addonBefore="M1" />
            <InputNumber min={1} value={params.kdj.m2} onChange={(value) => value && update('kdj', { ...params.kdj!, m2: value })} addonBefore="M2" />
            <Button danger type="text" onClick={() => remove('kdj')}>移除配置</Button>
          </Space>
        </ParamGroupCard>
      )}

      {params.boll && (
        <ParamGroupCard title="BOLL 覆盖">
          <Space>
            <InputNumber min={1} value={params.boll.n} onChange={(value) => value && update('boll', { ...params.boll!, n: value })} addonBefore="N" />
            <InputNumber min={0.1} step={0.1} value={params.boll.k} onChange={(value) => value && update('boll', { ...params.boll!, k: value })} addonBefore="K" />
            <Button danger type="text" onClick={() => remove('boll')}>移除配置</Button>
          </Space>
        </ParamGroupCard>
      )}

      {params.rsi && (
        <ParamGroupCard title="RSI 覆盖">
          <Space wrap>
            {params.rsi.map((value) => (
              <Tag key={value} closable onClose={() => update('rsi', params.rsi!.filter((item) => item !== value))} color="blue">{value}</Tag>
            ))}
            <InputNumber
              min={1}
              value={newPeriod.rsi ?? undefined}
              onChange={(value) => setNewPeriod((prev) => ({ ...prev, rsi: typeof value === 'number' ? value : null }))}
              placeholder="新周期"
            />
            <Button
              icon={<PlusOutlined />}
              onClick={() => {
                const value = newPeriod.rsi;
                if (!value || params.rsi!.includes(value)) return;
                update('rsi', [...params.rsi!, value].sort((a, b) => a - b));
                setNewPeriod((prev) => ({ ...prev, rsi: null }));
              }}
            >
              添加
            </Button>
            <Button danger type="text" onClick={() => remove('rsi')}>移除配置</Button>
          </Space>
        </ParamGroupCard>
      )}

      {inactiveKeys.length > 0 && (
        <Space wrap>
          <Text type="secondary">添加覆盖项：</Text>
          {inactiveKeys.map((option) => (
            <Button key={option.key} size="small" icon={<PlusOutlined />} onClick={() => update(option.key, structuredClone(defaults[option.key]))}>
              {option.label}
            </Button>
          ))}
        </Space>
      )}
    </Space>
  );
}

function DefaultsTab({ config, onChange }: { config: IndicatorConfig; onChange: (config: IndicatorConfig) => void }) {
  return (
    <IndicatorEditors
      params={config.defaults}
      onChange={(defaults) => onChange({ ...config, defaults })}
    />
  );
}

function CategoriesTab({ config, onChange }: { config: IndicatorConfig; onChange: (config: IndicatorConfig) => void }) {
  const [newName, setNewName] = useState('');
  const items = Object.entries(config.categories).map(([key, params]) => ({
    key,
    label: (
      <Flex justify="space-between" align="center" style={{ width: '100%' }}>
        <Space>
          <Text strong>{CATEGORY_LABELS[key] || key}</Text>
          <Text type="secondary" code>{key}</Text>
        </Space>
        <Button
          danger
          type="text"
          icon={<DeleteOutlined />}
          onClick={(event) => {
            event.stopPropagation();
            const next = { ...config.categories };
            delete next[key];
            onChange({ ...config, categories: next });
          }}
        />
      </Flex>
    ),
    children: (
      <PartialParamsEditor
        params={params}
        defaults={config.defaults}
        onChange={(nextParams) => onChange({
          ...config,
          categories: { ...config.categories, [key]: nextParams },
        })}
      />
    ),
  }));

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert type="info" showIcon message="优先级：个股覆盖 > 分类覆盖 > 默认参数" />
      <Collapse items={items} defaultActiveKey={items.map((item) => item.key)} />
      <Card size="small" title="新增分类覆盖">
        <Space>
          <Input
            value={newName}
            onChange={(event) => setNewName(event.target.value)}
            placeholder="分类名称，如 mid_cap"
            onPressEnter={() => {
              const name = newName.trim();
              if (!name || config.categories[name]) return;
              onChange({ ...config, categories: { ...config.categories, [name]: {} } });
              setNewName('');
            }}
            style={{ width: 280 }}
          />
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              const name = newName.trim();
              if (!name || config.categories[name]) return;
              onChange({ ...config, categories: { ...config.categories, [name]: {} } });
              setNewName('');
            }}
          >
            添加分类
          </Button>
        </Space>
      </Card>
    </Space>
  );
}

function OverridesTab({ config, onChange }: { config: IndicatorConfig; onChange: (config: IndicatorConfig) => void }) {
  const [newCode, setNewCode] = useState('');
  const items = Object.entries(config.overrides).map(([code, params]) => ({
    key: code,
    label: (
      <Flex justify="space-between" align="center" style={{ width: '100%' }}>
        <Space>
          <Text strong code>{code}</Text>
          <Text type="secondary">{Object.keys(params).length} 项覆盖</Text>
        </Space>
        <Button
          danger
          type="text"
          icon={<DeleteOutlined />}
          onClick={(event) => {
            event.stopPropagation();
            const next = { ...config.overrides };
            delete next[code];
            onChange({ ...config, overrides: next });
          }}
        />
      </Flex>
    ),
    children: (
      <PartialParamsEditor
        params={params}
        defaults={config.defaults}
        onChange={(nextParams) => onChange({
          ...config,
          overrides: { ...config.overrides, [code]: nextParams },
        })}
      />
    ),
  }));

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert type="info" showIcon message="个股覆盖优先级最高，适合处理少量特例股票。" />
      <Collapse items={items} defaultActiveKey={items.map((item) => item.key)} />
      <Card size="small" title="新增个股覆盖">
        <Space>
          <Input
            value={newCode}
            onChange={(event) => setNewCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
            placeholder="6位股票代码"
            onPressEnter={() => {
              if (!/^\d{6}$/.test(newCode) || config.overrides[newCode]) return;
              onChange({ ...config, overrides: { ...config.overrides, [newCode]: {} } });
              setNewCode('');
            }}
            style={{ width: 180 }}
          />
          <Button
            type="primary"
            icon={<PlusOutlined />}
            disabled={!/^\d{6}$/.test(newCode) || !!config.overrides[newCode]}
            onClick={() => {
              if (!/^\d{6}$/.test(newCode) || config.overrides[newCode]) return;
              onChange({ ...config, overrides: { ...config.overrides, [newCode]: {} } });
              setNewCode('');
            }}
          >
            添加个股
          </Button>
        </Space>
      </Card>
    </Space>
  );
}

function ParamReference() {
  const items = useMemo(() => ([
    { key: '1', label: 'ma', value: '[5, 10, 20, 60, 120]', desc: '均线周期列表' },
    { key: '2', label: 'macd', value: '12 / 26 / 9', desc: '快线 / 慢线 / 信号线周期' },
    { key: '3', label: 'kdj', value: '9 / 3 / 3', desc: 'RSV 周期 / K 平滑 / D 平滑' },
    { key: '4', label: 'boll', value: '20 / 2.0', desc: '均线周期 / 标准差倍数' },
    { key: '5', label: 'rsi', value: '[6, 12, 24]', desc: 'RSI 周期列表' },
  ]), []);

  return (
    <Collapse
      items={[
        {
          key: 'reference',
          label: (
            <Space>
              <InfoCircleOutlined />
              <span>参数说明</span>
            </Space>
          ),
          children: (
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              {items.map((item) => (
                <Flex key={item.key} justify="space-between" align="center" style={{ padding: '8px 0', borderBottom: '1px solid var(--ant-color-border-secondary)' }}>
                  <Space>
                    <Text code>{item.label}</Text>
                    <Text>{item.value}</Text>
                  </Space>
                  <Text type="secondary">{item.desc}</Text>
                </Flex>
              ))}
            </Space>
          ),
        },
      ]}
    />
  );
}

export default function SettingsPage() {
  const [config, setConfig] = useState<IndicatorConfig>(cloneIndicatorConfig(INITIAL_CONFIG));
  const [savedConfig, setSavedConfig] = useState<IndicatorConfig>(cloneIndicatorConfig(INITIAL_CONFIG));
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();

  const isDirty = useMemo(
    () => JSON.stringify(config) !== JSON.stringify(savedConfig),
    [config, savedConfig],
  );

  // Warn before leaving page with unsaved changes
  useEffect(() => {
    if (!isDirty) return;
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [isDirty]);

  const loadConfig = async () => {
    setLoading(true);
    try {
      const next = await api.indicatorSettings();
      setConfig(next);
      setSavedConfig(structuredClone(next));
    } catch (error) {
      const text = error instanceof Error ? error.message : '加载配置失败';
      messageApi.error(text);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadConfig();
  }, []);

  const handleSave = async () => {
    setSaving(true);
    try {
      const result = await api.saveIndicatorSettings(config);
      setConfig(result.config);
      setSavedConfig(structuredClone(result.config));
      messageApi.success('配置已保存');
    } catch (error) {
      const text = error instanceof Error ? error.message : '保存配置失败';
      messageApi.error(text);
    } finally {
      setSaving(false);
    }
  };

  const handleReload = () => {
    if (isDirty) {
      Modal.confirm({
        title: '确认重新加载？',
        icon: <ExclamationCircleOutlined />,
        content: '当前有未保存的修改，重新加载将丢弃这些改动。',
        okText: '丢弃并重新加载',
        okType: 'danger',
        cancelText: '取消',
        onOk: () => void loadConfig(),
      });
    } else {
      void loadConfig();
    }
  };

  const handleReset = () => {
    Modal.confirm({
      title: '确认恢复示例默认配置？',
      icon: <ExclamationCircleOutlined />,
      content: '这将把当前页面所有参数重置为内置示例值。重置后仍需点击"保存配置"才会写入服务端。',
      okText: '确认重置',
      cancelText: '取消',
      onOk: () => {
        setConfig(cloneIndicatorConfig(INITIAL_CONFIG));
        messageApi.info('已重置为示例配置，请确认后点击"保存配置"');
      },
    });
  };

  return (
    <>
      {contextHolder}
      <Space direction="vertical" size={16} style={{ width: '100%', maxWidth: 1200 }}>
        <div>
          <Title level={3} style={{ marginBottom: 0 }}>
            <Space>
              <SettingOutlined />
              配置
            </Space>
          </Title>
          <Paragraph type="secondary" style={{ marginBottom: 0 }}>
            指标参数配置 · <Text code>{config.path || '~/.tongstock/indicator.yaml'}</Text>
          </Paragraph>
        </div>

        <Alert
          type="info"
          showIcon
          message="当前页面已接入服务端配置"
          description="修改后将写入指标配置文件，并立即影响后续指标计算与筛选结果。"
        />

        {loading ? (
          <Card>
            <Skeleton active paragraph={{ rows: 12 }} />
          </Card>
        ) : (
          <>
            {isDirty && (
              <Alert
                type="warning"
                showIcon
                message="有未保存的修改"
                description="您已修改配置但尚未保存。离开页面或重新加载将丢失这些改动。"
              />
            )}

            <Tabs
              items={[
                {
                  key: 'defaults',
                  label: '默认参数',
                  children: <DefaultsTab config={config} onChange={setConfig} />,
                },
                {
                  key: 'categories',
                  label: '分类覆盖',
                  children: <CategoriesTab config={config} onChange={setConfig} />,
                },
                {
                  key: 'overrides',
                  label: '个股覆盖',
                  children: <OverridesTab config={config} onChange={setConfig} />,
                },
              ]}
            />

            <ParamReference />

            <Flex justify="space-between" align="center" wrap="wrap" gap={12}>
              <Space wrap>
                <Button type="primary" size="large" icon={<SaveOutlined />} loading={saving} onClick={handleSave} disabled={!isDirty}>
                  保存配置
                </Button>
                <Button icon={<ReloadOutlined />} onClick={handleReload}>
                  重新加载
                </Button>
              </Space>
              <Tooltip title="重置为内置示例默认配置，需手动点击保存才会写入服务端。">
                <Button onClick={handleReset}>
                  恢复示例默认配置
                </Button>
              </Tooltip>
            </Flex>
          </>
        )}
      </Space>
    </>
  );
}

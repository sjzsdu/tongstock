import { Card, Space, Tag, Typography, theme } from 'antd';

const { Text, Paragraph, Title } = Typography;

/** 四段流程各自的色相；具体色值在渲染时从主题色板取（见 STAGE_COLOR_STEP）。 */
type StageTone = 'blue' | 'purple' | 'cyan' | 'orange';

interface Stage {
  key: string;
  title: string;
  tone: StageTone;
  summary: string;
  detail: string;
}

/**
 * 每个色相取色板里「在深色卡片底（colorBgContainer）上仍达 4.5:1」的那一档。
 * 不能用 token.blue / token.purple 这类预设主色：它们是给浅色底准备的，
 * 紫色 #722ed1 压在深色卡片上只有 2.56:1（蓝 #1677ff 也只有 4.32:1）。
 */
const STAGE_COLOR_STEP = {
  blue: 'blue7',
  purple: 'purple8',
  cyan: 'cyan6',
  orange: 'orange6',
} as const;

/** 这套工具的四段式思考流程。顺序即依赖：后一步只接受前一步的产物。 */
const STAGES: Stage[] = [
  {
    key: 'research',
    title: '① 研究',
    tone: 'blue',
    summary: '把想法写成可编译的方法',
    detail: '入场条件、出场条件、失效条件必须写成机器可判定的规则，而不是一句主观描述。',
  },
  {
    key: 'verify',
    title: '② 验证',
    tone: 'purple',
    summary: '在冻结的真实历史上做样本外回测',
    detail: '训练段调参、样本外段检验；证据不达标的方法直接拒绝，绝不进入候选。',
  },
  {
    key: 'select',
    title: '③ 选股',
    tone: 'cyan',
    summary: '只让通过验证的方法判定今天',
    detail: '用今天的冻结快照执行已验证的方法；命中的股票才成为候选，并附上触发它的方法证据。',
  },
  {
    key: 'sell',
    title: '④ 卖出',
    tone: 'orange',
    summary: '按方法自带的退出计划离场',
    detail: '止损、止盈、最长持有天数与失效条件共同决定减仓或清仓，不靠临时情绪。',
  },
];

/**
 * P6・「这套工具怎么思考」的四段式图解。
 *
 * 用户懵的深层原因是：不懂「方法 → 证据 → 选股 → 卖出」这条机制。
 * 一张图把链条讲清楚，比任何口号都更能降低理解成本。
 */
export default function MethodMechanismDiagram() {
  const { token } = theme.useToken();
  const stageColor = (tone: StageTone) => token[STAGE_COLOR_STEP[tone]];

  return (
    <Card size="small" title="这套工具怎么思考">
      <Space direction="vertical" size={12} style={{ display: 'flex' }}>
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))',
            gap: 12,
          }}
        >
          {STAGES.map((stage, idx) => {
            const color = stageColor(stage.tone);
            return (
              <div
                key={stage.key}
                style={{
                  position: 'relative',
                  border: `1px solid ${color}33`,
                  borderTop: `3px solid ${color}`,
                  borderRadius: 8,
                  padding: '10px 12px',
                  background: `${color}0a`,
                }}
              >
                <Space direction="vertical" size={4} style={{ display: 'flex' }}>
                  <Text strong style={{ color }}>
                    {stage.title}
                  </Text>
                  <Text style={{ fontSize: 13 }}>{stage.summary}</Text>
                  <Text type="secondary" style={{ fontSize: 12, lineHeight: 1.5 }}>
                    {stage.detail}
                  </Text>
                </Space>
                {idx < STAGES.length - 1 && (
                  <span
                    aria-hidden
                    style={{
                      position: 'absolute',
                      right: -10,
                      top: '50%',
                      transform: 'translateY(-50%)',
                      // 装饰性箭头：走主题的次级文字色，别写死浅灰（深色底上要么刺眼要么看不见）
                      color: token.colorTextSecondary,
                      fontSize: 14,
                    }}
                  >
                    →
                  </span>
                )}
              </div>
            );
          })}
        </div>

        <div
          style={{
            // 深色主题下不能出现写死的浅色块：原来的 #fafafa 底 + 默认浅色文字
            // 等于白底白字（1.05:1），整段「一个真实例子」几乎不可读。
            border: `1px dashed ${token.colorBorder}`,
            borderRadius: 8,
            padding: '10px 12px',
            background: token.colorBgLayout,
          }}
        >
          <Title level={5} style={{ marginTop: 0, marginBottom: 6 }}>
            一个真实例子
          </Title>
          <Paragraph style={{ margin: 0, fontSize: 13, lineHeight: 1.7 }}>
            以「均线多头排列」为例：<Text strong>研究</Text> 阶段把它写成
            「收盘价站上 ma5，且 ma5 &gt; ma10 &gt; ma20」；<Text strong>验证</Text>{' '}
            阶段在冻结的真实日线上做样本外回测，得到夏普比率为负、胜率不足 30% 的结论——
            系统据此把它标为 <Tag color="red">未通过证据门槛</Tag>，
            它<Text strong>不会</Text>进入选股；因此今日候选里不会出现它。
            这不是故障，而是「证据不足就不推荐」在起作用。
          </Paragraph>
        </div>
      </Space>
    </Card>
  );
}
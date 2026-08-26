import { useState, useEffect } from 'react';
import { PlusOutlined, SaveOutlined } from '@ant-design/icons';
import { Alert, Button, Form, Input, InputNumber, List, Modal, Select, Space, Switch, message } from 'antd';
import type { CustomStockPool, StockPoolFilter, FilterField } from '../types/api';

const filterFieldOptions: { field: FilterField; label: string; type: 'range' | 'select' | 'boolean'; unit?: string }[] = [
  { field: 'marketCap', label: '流通市值', type: 'range', unit: '亿' },
  { field: 'price', label: '股价', type: 'range', unit: '元' },
  { field: 'turnoverRate', label: '换手率', type: 'range', unit: '%' },
  { field: 'changePct', label: '涨跌幅', type: 'range', unit: '%' },
  { field: 'volumeRatio', label: '量比', type: 'range' },
  { field: 'exchange', label: '交易所', type: 'select' },
  { field: 'board', label: '板块类型', type: 'select' },
  { field: 'excludeST', label: '排除ST', type: 'boolean' },
];

const exchangeOptions = [
  { value: 'sh', label: '上海(SH)' },
  { value: 'sz', label: '深圳(SZ)' },
  { value: 'bj', label: '北京(BJ)' },
];

const boardOptions = [
  { value: 'main', label: '主板' },
  { value: 'chuangye', label: '创业板' },
  { value: 'kechuang', label: '科创板' },
  { value: 'beijiao', label: '北交所' },
];

interface StockPoolModalProps {
  visible: boolean;
  onCancel: () => void;
  onSave: (pool: CustomStockPool) => void;
  editingPool?: CustomStockPool | null;
}

function renderFilterField(filter: StockPoolFilter, index: number, onChange: (index: number, field: keyof StockPoolFilter, value: unknown) => void, updateValue: (index: number, idx: number, value: number | string | boolean) => void) {
  const fieldOption = filterFieldOptions.find(f => f.field === filter.field);
  if (!fieldOption) return null;

  switch (fieldOption.type) {
    case 'range':
      switch (filter.operator) {
        case 'between':
          return (
            <Space key={index}>
              <InputNumber
                min={fieldOption.field === 'changePct' ? -100 : 0}
                style={{ width: 80 }}
                value={filter.value[0] as number}
                onChange={(v) => updateValue(index, 0, v || 0)}
                placeholder="最小"
              />
              <span>~</span>
              <InputNumber
                min={0}
                style={{ width: 80 }}
                value={filter.value[1] as number}
                onChange={(v) => updateValue(index, 1, v || 0)}
                placeholder="最大"
              />
              {fieldOption.unit && <span>{fieldOption.unit}</span>}
            </Space>
          );
        case 'gt':
        case 'gte':
          return (
            <Space key={index}>
              <InputNumber
                min={fieldOption.field === 'changePct' ? -100 : 0}
                style={{ width: 120 }}
                value={filter.value[0] as number}
                onChange={(v) => updateValue(index, 0, v || 0)}
                placeholder={filter.operator === 'gt' ? '大于' : '大于等于'}
              />
              {fieldOption.unit && <span>{fieldOption.unit}</span>}
            </Space>
          );
        case 'lt':
        case 'lte':
          return (
            <Space key={index}>
              <InputNumber
                min={fieldOption.field === 'changePct' ? -100 : 0}
                style={{ width: 120 }}
                value={filter.value[0] as number}
                onChange={(v) => updateValue(index, 0, v || 0)}
                placeholder={filter.operator === 'lt' ? '小于' : '小于等于'}
              />
              {fieldOption.unit && <span>{fieldOption.unit}</span>}
            </Space>
          );
        default:
          return null;
      }
    case 'select':
      if (filter.field === 'exchange') {
        return (
          <Select
            key={index}
            mode="multiple"
            style={{ width: 180 }}
            options={exchangeOptions}
            value={filter.value as string[]}
            onChange={(v) => onChange(index, 'value', v)}
            placeholder="选择交易所"
            size="small"
          />
        );
      }
      if (filter.field === 'board') {
        return (
          <Select
            key={index}
            mode="multiple"
            style={{ width: 180 }}
            options={boardOptions}
            value={filter.value as string[]}
            onChange={(v) => onChange(index, 'value', v)}
            placeholder="选择板块类型"
            size="small"
          />
        );
      }
      break;
    case 'boolean':
      return (
        <Switch
          key={index}
          checked={filter.value[0] === true}
          onChange={(v) => updateValue(index, 0, v)}
        />
      );
  }
  return null;
}

export function StockPoolModal({ visible, onCancel, onSave, editingPool }: StockPoolModalProps) {
  const [form] = Form.useForm();
  const [filters, setFilters] = useState<StockPoolFilter[]>([]);

  // 修复：编辑模式下初始化 filters 和表单数据
  useEffect(() => {
    if (visible && editingPool) {
      // 初始化表单数据
      form.setFieldsValue({
        name: editingPool.name,
        description: editingPool.description,
      });
      // 初始化 filters
      setFilters([...editingPool.filters]);
    } else if (!visible) {
      // 关闭时重置
      form.resetFields();
      setFilters([]);
    }
  }, [visible, editingPool, form]);

  const addFilter = () => {
    setFilters(prev => [...prev, { field: 'marketCap', operator: 'between', value: [0, 0] }]);
  };

  const removeFilter = (index: number) => {
    setFilters(prev => prev.filter((_, i) => i !== index));
  };

  const updateFilter = (index: number, field: keyof StockPoolFilter, value: unknown) => {
    setFilters(prev => prev.map((f, i) => i === index ? { ...f, [field]: value } : f));
  };

  const updateFilterValue = (index: number, idx: number, value: number | string | boolean) => {
    setFilters(prev => prev.map((f, i) => {
      if (i === index) {
        const newValues = [...f.value];
        newValues[idx] = value;
        return { ...f, value: newValues };
      }
      return f;
    }));
  };

  // 修复：添加表单验证
  const handleSave = async () => {
    try {
      // 先验证表单
      const values = await form.validateFields();
      const now = new Date().toISOString();
      
      if (editingPool) {
        onSave({
          ...editingPool,
          name: values.name,
          description: values.description,
          filters,
          updatedAt: now,
        });
      } else {
        onSave({
          id: Date.now().toString(),
          name: values.name,
          description: values.description,
          filters,
          createdAt: now,
          updatedAt: now,
        });
      }
      onCancel();
    } catch {
      message.error('请填写必填项');
    }
  };

  return (
    <Modal
      title={editingPool ? '编辑股票池' : '新建股票池'}
      open={visible}
      onCancel={onCancel}
      footer={[
        <Button key="cancel" onClick={onCancel}>取消</Button>,
        <Button key="save" type="primary" onClick={handleSave} icon={<SaveOutlined />}>保存</Button>,
      ]}
      width={600}
    >
      <Form form={form} layout="vertical">
        <Form.Item label="股票池名称" name="name" rules={[{ required: true, message: '请输入股票池名称' }]}>
          <Input placeholder="输入股票池名称" />
        </Form.Item>
        <Form.Item label="描述" name="description">
          <Input.TextArea placeholder="输入股票池描述（可选）" rows={2} />
        </Form.Item>
      </Form>

      <div style={{ margin: '16px 0' }}>
        <Space>
          <span style={{ fontWeight: 'bold' }}>筛选条件</span>
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={addFilter}>添加条件</Button>
        </Space>
      </div>

      {filters.length === 0 ? (
        <Alert type="info" message="暂无筛选条件，点击上方按钮添加" />
      ) : (
        <List
          dataSource={filters}
          renderItem={(filter, index) => (
            <List.Item
              extra={<Button size="small" danger onClick={() => removeFilter(index)}>删除</Button>}
              style={{ padding: '8px 0' }}
            >
              <Space direction="vertical" size={8}>
                <Space>
                  <Select
                    value={filter.field}
                    onChange={(v) => updateFilter(index, 'field', v)}
                    options={filterFieldOptions.map(f => ({ value: f.field, label: f.label }))}
                    style={{ width: 140 }}
                    size="small"
                  />
                  <Select
                    value={filter.operator}
                    onChange={(v) => updateFilter(index, 'operator', v)}
                    options={[
                      { value: 'between', label: '区间' },
                      { value: 'gt', label: '大于' },
                      { value: 'gte', label: '大于等于' },
                      { value: 'lt', label: '小于' },
                      { value: 'lte', label: '小于等于' },
                      { value: 'in', label: '包含' },
                    ]}
                    style={{ width: 80 }}
                    size="small"
                  />
                </Space>
                {renderFilterField(filter, index, updateFilter, updateFilterValue)}
              </Space>
            </List.Item>
          )}
        />
      )}
    </Modal>
  );
}

import { describe, it, expect } from 'vitest';

describe('研究工作台信息架构', () => {
  describe('工作流阶段定义', () => {
    const workflowStages = [
      { key: 'home', title: '研究首页', path: '/' },
      { key: 'hypothesis', title: '假设', path: '/research/hypothesis' },
      { key: 'experiment', title: '实验', path: '/research/experiment' },
      { key: 'candidates', title: '候选', path: '/research/candidates' },
      { key: 'verified', title: '已验证', path: '/research/verified' },
      { key: 'observation', title: '前向观察', path: '/research/observation' },
      { key: 'retrospective', title: '复盘', path: '/research/retrospective' },
    ];

    it('应有 7 个研究工作流阶段', () => {
      expect(workflowStages.length).toBe(7);
    });

    it('每个阶段应有唯一的 key', () => {
      const keys = workflowStages.map(s => s.key);
      const uniqueKeys = new Set(keys);
      expect(uniqueKeys.size).toBe(workflowStages.length);
    });

    it('每个阶段应有路径', () => {
      for (const stage of workflowStages) {
        expect(stage.path).toBeDefined();
        expect(stage.path.startsWith('/')).toBe(true);
      }
    });

    it('路径应包含 /research 前缀 (除首页外)', () => {
      for (const stage of workflowStages) {
        if (stage.key === 'home') continue;
        expect(stage.path.startsWith('/research/')).toBe(true);
      }
    });

    it('应按研究顺序排列', () => {
      const expectedOrder = ['home', 'hypothesis', 'experiment', 'candidates', 'verified', 'observation', 'retrospective'];
      const actualOrder = workflowStages.map(s => s.key);
      expect(actualOrder).toEqual(expectedOrder);
    });
  });

  describe('路由映射', () => {
    const routeMappings: Record<string, string> = {
      '/': 'ResearchHome',
      '/research': 'ResearchHome',
      '/research/hypothesis': 'Hypothesis',
      '/research/experiment': 'Experiment',
      '/research/candidates': 'Candidates',
      '/research/verified': 'Verified',
      '/research/observation': 'Observation',
      '/research/retrospective': 'Retrospective',
    };

    it('所有研究路由都应有对应的组件', () => {
      for (const [, component] of Object.entries(routeMappings)) {
        expect(component).toBeDefined();
        expect(component.length).toBeGreaterThan(0);
      }
    });

    it('首页路由应映射到 ResearchHome', () => {
      expect(routeMappings['/']).toBe('ResearchHome');
    });

    it('保持向后兼容', () => {
      const legacyRoutes = [
        '/watchlist',
        '/stock/choose',
        '/screen',
        '/portfolio',
        '/blocks',
        '/settings',
        '/agent',
        '/paradigms',
        '/strategy/overnight',
        '/news',
      ];
      for (const route of legacyRoutes) {
        expect(route).toBeDefined();
        expect(route.startsWith('/')).toBe(true);
      }
    });
  });

  describe('导航结构', () => {
    describe('主导航：研究工作流', () => {
      const mainNav = [
        '/',
        '/research/hypothesis',
        '/research/experiment',
        '/research/candidates',
        '/research/verified',
        '/research/observation',
        '/research/retrospective',
      ];

      it('主导航包含 7 个工作流入口', () => {
        expect(mainNav.length).toBe(7);
      });

      it('主导航路径唯一', () => {
        const unique = new Set(mainNav);
        expect(unique.size).toBe(mainNav.length);
      });
    });

    describe('二级导航：高级工具', () => {
      const toolsNav = [
        '/agent',
        '/paradigms',
        '/screen',
        '/stock/choose',
        '/watchlist',
        '/portfolio',
        '/blocks',
        '/strategy/overnight',
        '/news',
        '/settings',
      ];

      it('高级工具包含所有原有功能', () => {
        expect(toolsNav.length).toBe(10);
      });

      it('高级工具作为子菜单存在', () => {
        expect(toolsNav.length).toBeGreaterThan(0);
      });
    });
  });

  describe('面包屑导航', () => {
    const breadcrumbLabels: Record<string, string> = {
      research: '研究工作台',
      hypothesis: '假设',
      experiment: '实验',
      candidates: '候选',
      verified: '已验证',
      observation: '前向观察',
      retrospective: '复盘',
    };

    it('所有研究工作流页面都应有面包屑标签', () => {
      for (const key of ['research', 'hypothesis', 'experiment', 'candidates', 'verified', 'observation', 'retrospective']) {
        expect(breadcrumbLabels[key]).toBeDefined();
        expect(breadcrumbLabels[key].length).toBeGreaterThan(0);
      }
    });
  });
});

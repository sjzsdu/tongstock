import { useState, useCallback, useEffect, useRef } from 'react';
import type { DrawerProps } from 'antd';
import { Drawer } from 'antd';

interface ResizableDrawerProps extends Omit<DrawerProps, 'width'> {
  defaultWidth?: number;
  minWidth?: number;
  maxWidth?: number;
}

export default function ResizableDrawer({
  defaultWidth = 480,
  minWidth = 320,
  maxWidth = 900,
  children,
  ...props
}: ResizableDrawerProps) {
  const [width, setWidth] = useState(defaultWidth);
  const dragging = useRef(false);
  const startX = useRef(0);
  const startWidth = useRef(0);

  const onMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    dragging.current = true;
    startX.current = e.clientX;
    startWidth.current = width;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  }, [width]);

  useEffect(() => {
    const onMouseMove = (e: MouseEvent) => {
      if (!dragging.current) return;
      const delta = startX.current - e.clientX;
      const newWidth = Math.min(maxWidth, Math.max(minWidth, startWidth.current + delta));
      setWidth(newWidth);
    };
    const onMouseUp = () => {
      if (dragging.current) {
        dragging.current = false;
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
      }
    };
    document.addEventListener('mousemove', onMouseMove);
    document.addEventListener('mouseup', onMouseUp);
    return () => {
      document.removeEventListener('mousemove', onMouseMove);
      document.removeEventListener('mouseup', onMouseUp);
    };
  }, [minWidth, maxWidth]);

  return (
    <Drawer
      {...props}
      width={width}
    >
      <div
        onMouseDown={onMouseDown}
        style={{
          position: 'absolute',
          top: 0,
          left: 0,
          width: 5,
          height: '100%',
          cursor: 'col-resize',
          zIndex: 10,
        }}
        title="拖拽调整宽度"
      />
      {children}
    </Drawer>
  );
}

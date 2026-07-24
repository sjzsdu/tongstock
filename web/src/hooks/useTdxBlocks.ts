import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';

export interface BlockFile {
  file: string;
  name: string;
  desc: string;
}

export interface BlockInfo {
  name: string;
  type: number;
  count: number;
}

export interface BlockStock {
  code: string;
  name: string;
  exchange: string;
}

interface UseTdxBlocksReturn {
  files: BlockFile[];
  selectedFile: string;
  setSelectedFile: (file: string) => void;
  blocks: BlockInfo[];
  selectedBlock: string | null;
  setSelectedBlock: (block: string | null) => void;
  blockStocks: BlockStock[];
  loadingFiles: boolean;
  loadingBlocks: boolean;
  loadingStocks: boolean;
}

export function useTdxBlocks(): UseTdxBlocksReturn {
  const [files, setFiles] = useState<BlockFile[]>([]);
  const [selectedFile, setSelectedFile] = useState('block_fg.dat');
  const [blocks, setBlocks] = useState<BlockInfo[]>([]);
  const [selectedBlock, setSelectedBlock] = useState<string | null>(null);
  const [blockStocks, setBlockStocks] = useState<BlockStock[]>([]);
  const [loadingFiles, setLoadingFiles] = useState(true);
  const [loadingBlocks, setLoadingBlocks] = useState(false);
  const [loadingStocks, setLoadingStocks] = useState(false);

  const loadFiles = useCallback(async () => {
    setLoadingFiles(true);
    try {
      const result = await api.blockFiles();
      setFiles(result.files);
      if (result.files.length > 0) {
        setSelectedFile(result.files[0].file);
      }
    } finally {
      setLoadingFiles(false);
    }
  }, []);

  const loadBlocks = useCallback(async () => {
    setLoadingBlocks(true);
    setSelectedBlock(null);
    setBlockStocks([]);
    try {
      const result = await api.blockList(selectedFile);
      setBlocks(result.blocks);
    } finally {
      setLoadingBlocks(false);
    }
  }, [selectedFile]);

  const loadBlockStocks = useCallback(async () => {
    if (!selectedBlock) return;
    
    setLoadingStocks(true);
    try {
      const result = await api.blockShow(selectedBlock, undefined, selectedFile);
      setBlockStocks(result.stocks || []);
    } finally {
      setLoadingStocks(false);
    }
  }, [selectedBlock, selectedFile]);

  useEffect(() => {
    void loadFiles();
  }, [loadFiles]);

  useEffect(() => {
    void loadBlocks();
  }, [loadBlocks]);

  useEffect(() => {
    void loadBlockStocks();
  }, [loadBlockStocks]);

  return {
    files,
    selectedFile,
    setSelectedFile,
    blocks,
    selectedBlock,
    setSelectedBlock,
    blockStocks,
    loadingFiles,
    loadingBlocks,
    loadingStocks,
  };
}
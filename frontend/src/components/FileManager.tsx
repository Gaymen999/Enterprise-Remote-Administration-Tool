import { useState, useCallback } from 'react';
import { 
  Folder, File, FileText, Image, FileCode, Archive, 
  ChevronRight, ChevronDown, Upload, Download, Trash2, 
  FolderPlus, RefreshCw, Home, ArrowLeft, Eye, EyeOff
} from 'lucide-react';
import { api } from '../services/api';

interface FileItem {
  name: string;
  size: number;
  mode: string;
  mod_time: string;
  is_dir: boolean;
  is_link: boolean;
}

interface FileOperationResult {
  success: boolean;
  data?: FileItem[] | FileDownloadData;
  error?: string;
  request_id: string;
}

interface FileDownloadData {
  content: string;
  size: number;
  name: string;
  original_path: string;
}

interface FileManagerProps {
  agentId: string;
}

export function FileManager({ agentId }: FileManagerProps) {
  const [currentPath, setCurrentPath] = useState<string>('');
  const [files, setFiles] = useState<FileItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<'list' | 'grid'>('list');
  const [showHidden, setShowHidden] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);

  const fetchDirectory = useCallback(async (path: string) => {
    setLoading(true);
    setError(null);
    try {
      const response = await api.post('/file/operation', {
        agent_id: agentId,
        operation: 'list',
        path: path,
      });
      
      if (response.data.files) {
        const sortedFiles = response.data.files.sort((a: FileItem, b: FileItem) => {
          if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
          return a.name.localeCompare(b.name);
        });
        setFiles(sortedFiles);
      }
      setCurrentPath(path);
    } catch (err) {
      setError('Failed to fetch directory');
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, [agentId]);

  const navigateTo = useCallback((path: string) => {
    fetchDirectory(path);
    setSelectedFile(null);
  }, [fetchDirectory]);

  const navigateUp = useCallback(() => {
    const parentPath = currentPath.split(/[/\\]/).slice(0, -1).join('/') || '/';
    navigateTo(parentPath);
  }, [currentPath, navigateTo]);

  const handleFileClick = (file: FileItem) => {
    if (file.is_dir) {
      navigateTo(currentPath.endsWith('/') || currentPath.endsWith('\\') 
        ? currentPath + file.name 
        : currentPath + '/' + file.name);
    } else {
      setSelectedFile(file.name);
    }
  };

  const handleDelete = async (fileName: string) => {
    if (!confirm(`Delete "${fileName}"?`)) return;

    try {
      await api.post('/file/operation', {
        agent_id: agentId,
        operation: 'delete',
        path: currentPath.endsWith('/') || currentPath.endsWith('\\')
          ? currentPath + fileName
          : currentPath + '/' + fileName,
      });
      fetchDirectory(currentPath);
    } catch (err) {
      setError('Failed to delete');
      console.error(err);
    }
  };

  const handleDownload = async (fileName: string) => {
    try {
      const response = await api.post('/file/operation', {
        agent_id: agentId,
        operation: 'download',
        path: currentPath.endsWith('/') || currentPath.endsWith('\\')
          ? currentPath + fileName
          : currentPath + '/' + fileName,
      });

      if (response.data?.data?.content) {
        const link = document.createElement('a');
        link.href = `data:application/octet-stream;base64,${response.data.data.content}`;
        link.download = response.data.data.name;
        link.click();
      }
    } catch (err) {
      setError('Failed to download');
      console.error(err);
    }
  };

  const handleCreateFolder = async () => {
    const folderName = prompt('Enter folder name:');
    if (!folderName) return;

    try {
      await api.post('/file/operation', {
        agent_id: agentId,
        operation: 'mkdir',
        path: currentPath.endsWith('/') || currentPath.endsWith('\\')
          ? currentPath + folderName
          : currentPath + '/' + folderName,
      });
      fetchDirectory(currentPath);
    } catch (err) {
      setError('Failed to create folder');
      console.error(err);
    }
  };

  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    setUploadProgress(0);
    const formData = new FormData();
    formData.append('path', currentPath);
    formData.append('file', file);

    try {
      await api.post(`/file/upload/${agentId}`, formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
        onUploadProgress: (progressEvent) => {
          setUploadProgress(progressEvent.loaded / progressEvent.total * 100);
        },
      });
      fetchDirectory(currentPath);
    } catch (err) {
      setError('Failed to upload');
      console.error(err);
    } finally {
      setUploadProgress(null);
      event.target.value = '';
    }
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const getFileIcon = (file: FileItem) => {
    if (file.is_dir) return <Folder className="w-5 h-5 text-yellow-500" />;
    if (file.is_link) return <File className="w-5 h-5 text-purple-500" />;
    
    const ext = file.name.split('.').pop()?.toLowerCase();
    switch (ext) {
      case 'txt': case 'md': case 'log':
        return <FileText className="w-5 h-5 text-gray-400" />;
      case 'jpg': case 'jpeg': case 'png': case 'gif': case 'svg':
        return <Image className="w-5 h-5 text-green-500" />;
      case 'js': case 'ts': case 'py': case 'go': case 'rs':
        return <FileCode className="w-5 h-5 text-blue-500" />;
      case 'zip': case 'tar': case 'gz': case 'rar':
        return <Archive className="w-5 h-5 text-orange-500" />;
      default:
        return <File className="w-5 h-5 text-gray-400" />;
    }
  };

  const filteredFiles = showHidden 
    ? files 
    : files.filter(f => !f.name.startsWith('.'));

  return (
    <div className="flex flex-col h-full bg-gray-900">
      <div className="p-3 bg-gray-800 border-b border-gray-700 flex items-center gap-2">
        <button
          onClick={() => navigateTo('')}
          className="p-2 hover:bg-gray-700 rounded"
          title="Home"
        >
          <Home className="w-4 h-4 text-gray-400" />
        </button>
        
        <button
          onClick={navigateUp}
          disabled={!currentPath}
          className="p-2 hover:bg-gray-700 rounded disabled:opacity-50"
          title="Go up"
        >
          <ArrowLeft className="w-4 h-4 text-gray-400" />
        </button>

        <button
          onClick={() => fetchDirectory(currentPath)}
          className="p-2 hover:bg-gray-700 rounded"
          title="Refresh"
        >
          <RefreshCw className={`w-4 h-4 text-gray-400 ${loading ? 'animate-spin' : ''}`} />
        </button>

        <div className="flex-1 px-3 text-sm text-gray-400 truncate">
          {currentPath || '/'}
        </div>

        <button
          onClick={handleCreateFolder}
          className="p-2 hover:bg-gray-700 rounded"
          title="New Folder"
        >
          <FolderPlus className="w-4 h-4 text-gray-400" />
        </button>

        <label className="p-2 hover:bg-gray-700 rounded cursor-pointer">
          <Upload className="w-4 h-4 text-gray-400" />
          <input
            type="file"
            className="hidden"
            onChange={handleUpload}
          />
        </label>

        <button
          onClick={() => setShowHidden(!showHidden)}
          className="p-2 hover:bg-gray-700 rounded"
          title={showHidden ? 'Hide hidden files' : 'Show hidden files'}
        >
          {showHidden ? <Eye className="w-4 h-4 text-gray-400" /> : <EyeOff className="w-4 h-4 text-gray-400" />}
        </button>
      </div>

      {uploadProgress !== null && (
        <div className="h-1 bg-gray-700">
          <div 
            className="h-full bg-blue-500 transition-all"
            style={{ width: `${uploadProgress}%` }}
          />
        </div>
      )}

      {error && (
        <div className="p-3 bg-red-500/20 border-b border-red-500/50 text-red-400 text-sm">
          {error}
        </div>
      )}

      <div className="flex-1 overflow-auto">
        {loading ? (
          <div className="flex items-center justify-center h-full text-gray-400">
            Loading...
          </div>
        ) : filteredFiles.length === 0 ? (
          <div className="flex items-center justify-center h-full text-gray-400">
            Empty directory
          </div>
        ) : (
          <table className="w-full">
            <thead className="bg-gray-800 sticky top-0">
              <tr className="text-left text-xs text-gray-400 uppercase">
                <th className="px-4 py-2 w-8"></th>
                <th className="px-4 py-2">Name</th>
                <th className="px-4 py-2 w-24">Size</th>
                <th className="px-4 py-2 w-40">Modified</th>
                <th className="px-4 py-2 w-24">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filteredFiles.map((file, index) => (
                <tr
                  key={index}
                  className="border-b border-gray-800 hover:bg-gray-800/50 cursor-pointer"
                  onClick={() => handleFileClick(file)}
                >
                  <td className="px-4 py-2 text-center">
                    {file.is_dir && (
                      <ChevronRight className="w-4 h-4 text-gray-500" />
                    )}
                  </td>
                  <td className="px-4 py-2">
                    <div className="flex items-center gap-2">
                      {getFileIcon(file)}
                      <span className="text-white truncate">{file.name}</span>
                    </div>
                  </td>
                  <td className="px-4 py-2 text-gray-400 text-sm">
                    {file.is_dir ? '--' : formatFileSize(file.size)}
                  </td>
                  <td className="px-4 py-2 text-gray-400 text-sm">
                    {file.mod_time}
                  </td>
                  <td className="px-4 py-2" onClick={(e) => e.stopPropagation()}>
                    <div className="flex gap-1">
                      {!file.is_dir && (
                        <button
                          onClick={() => handleDownload(file.name)}
                          className="p-1 hover:bg-gray-700 rounded"
                          title="Download"
                        >
                          <Download className="w-4 h-4 text-gray-400" />
                        </button>
                      )}
                      <button
                        onClick={() => handleDelete(file.name)}
                        className="p-1 hover:bg-gray-700 rounded"
                        title="Delete"
                      >
                        <Trash2 className="w-4 h-4 text-red-400" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="p-2 bg-gray-800 border-t border-gray-700 text-xs text-gray-500">
        {filteredFiles.length} items | {formatFileSize(files.reduce((acc, f) => acc + f.size, 0))}
      </div>
    </div>
  );
}

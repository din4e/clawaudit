import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import type { ScansState, Scan } from '@/types/models';
import type { ScanStatusResponse } from '@/types/api';
import { apiClient } from '@/lib/api';

const initialState: ScansState = {
  scans: [],
  selectedScanId: null,
  total: 0,
  filters: {
    limit: 50,
    offset: 0,
  },
};

// Async thunks
export const fetchScans = createAsyncThunk(
  'scans/fetchScans',
  async (params: { status?: string; limit?: number; offset?: number } = {}) => {
    const queryParams = new URLSearchParams();
    if (params.status) queryParams.append('status', params.status);
    queryParams.append('limit', String(params.limit || 50));
    queryParams.append('offset', String(params.offset || 0));

    const response = await apiClient.get<{ scans: Scan[]; total: number }>(
      `/scans?${queryParams.toString()}`
    );
    return response;
  }
);

export const selectScan = createAsyncThunk(
  'scans/selectScan',
  async (scanId: string, { dispatch }) => {
    // Trigger status poll
    dispatch(pollScanStatus(scanId));
    return scanId;
  }
);

export const pollScanStatus = createAsyncThunk(
  'scans/pollStatus',
  async (scanId: string) => {
    const response = await apiClient.get<ScanStatusResponse>(`/scan/${scanId}/status`);
    return { scanId, status: response };
  }
);

export const deleteScanAsync = createAsyncThunk(
  'scans/deleteScan',
  async (scanId: string) => {
    await apiClient.delete(`/scan/${scanId}`);
    return scanId;
  }
);

const scansSlice = createSlice({
  name: 'scans',
  initialState,
  reducers: {
    setSelectedScanId: (state, action: PayloadAction<string | null>) => {
      state.selectedScanId = action.payload;
    },
    clearSelectedScan: (state) => {
      state.selectedScanId = null;
    },
    setFilters: (state, action: PayloadAction<Partial<ScansState['filters']>>) => {
      state.filters = { ...state.filters, ...action.payload };
    },
    updateScanInList: (state, action: PayloadAction<Partial<Scan> & { id: string }>) => {
      const index = state.scans.findIndex(s => s.id === action.payload.id);
      if (index !== -1) {
        state.scans[index] = { ...state.scans[index], ...action.payload };
      }
    },
    addScanToList: (state, action: PayloadAction<Scan>) => {
      const existingIndex = state.scans.findIndex(s => s.id === action.payload.id);
      if (existingIndex !== -1) {
        state.scans[existingIndex] = action.payload;
      } else {
        state.scans.unshift(action.payload);
      }
    },
  },
  extraReducers: (builder) => {
    builder
      // fetchScans
      .addCase(fetchScans.pending, (state) => {
        // Could set loading state here
      })
      .addCase(fetchScans.fulfilled, (state, action) => {
        state.scans = action.payload.scans;
        state.total = action.payload.total;
      })
      .addCase(fetchScans.rejected, (state) => {
        // Could handle error here
      })
      // selectScan
      .addCase(selectScan.fulfilled, (state, action) => {
        state.selectedScanId = action.payload;
      })
      // pollScanStatus
      .addCase(pollScanStatus.fulfilled, (state, action) => {
        const { scanId, status } = action.payload;
        const index = state.scans.findIndex(s => s.id === scanId);
        if (index !== -1) {
          state.scans[index] = {
            ...state.scans[index],
            status: status.status,
            progress: status.progress,
            message: status.message,
          };
        }
      })
      // deleteScanAsync
      .addCase(deleteScanAsync.fulfilled, (state, action) => {
        state.scans = state.scans.filter(s => s.id !== action.payload);
        if (state.selectedScanId === action.payload) {
          state.selectedScanId = null;
        }
        state.total = Math.max(0, state.total - 1);
      });
  },
});

export const {
  setSelectedScanId,
  clearSelectedScan,
  setFilters,
  updateScanInList,
  addScanToList,
} = scansSlice.actions;

export default scansSlice.reducer;

// Selectors
export const selectAllScans = (state: { scans: ScansState }) => state.scans.scans;
export const selectSelectedScanId = (state: { scans: ScansState }) => state.scans.selectedScanId;
export const selectSelectedScan = (state: { scans: ScansState }) => {
  const scanId = state.scans.selectedScanId;
  return scanId ? state.scans.scans.find(s => s.id === scanId) : null;
};
export const selectScansTotal = (state: { scans: ScansState }) => state.scans.total;
export const selectScansFilters = (state: { scans: ScansState }) => state.scans.filters;

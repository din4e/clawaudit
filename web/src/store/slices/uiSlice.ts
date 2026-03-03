import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import type { UiState, ActiveModal, IssuesFilters } from '@/types/models';

const initialState: UiState = {
  activeModal: null,
  modalData: {},
  filters: {},
  sidebarCollapsed: false,
};

const uiSlice = createSlice({
  name: 'ui',
  initialState,
  reducers: {
    openModal: (state, action: PayloadAction<{ modal: ActiveModal; data?: Partial<UiState['modalData']> }>) => {
      state.activeModal = action.payload.modal;
      if (action.payload.data) {
        state.modalData = { ...state.modalData, ...action.payload.data };
      }
    },
    closeModal: (state) => {
      state.activeModal = null;
    },
    updateModalData: (state, action: PayloadAction<Partial<UiState['modalData']>>) => {
      state.modalData = { ...state.modalData, ...action.payload };
    },
    setIssuesFilters: (state, action: PayloadAction<Partial<IssuesFilters>>) => {
      state.filters = { ...state.filters, ...action.payload };
    },
    clearIssuesFilters: (state) => {
      state.filters = {};
    },
    toggleSidebar: (state) => {
      state.sidebarCollapsed = !state.sidebarCollapsed;
    },
    setSidebarCollapsed: (state, action: PayloadAction<boolean>) => {
      state.sidebarCollapsed = action.payload;
    },
  },
});

export const {
  openModal,
  closeModal,
  updateModalData,
  setIssuesFilters,
  clearIssuesFilters,
  toggleSidebar,
  setSidebarCollapsed,
} = uiSlice.actions;

export default uiSlice.reducer;

// Selectors
export const selectActiveModal = (state: { ui: UiState }) => state.ui.activeModal;
export const selectModalData = (state: { ui: UiState }) => state.ui.modalData;
export const selectIssuesFilters = (state: { ui: UiState }) => state.ui.filters;
export const selectSidebarCollapsed = (state: { ui: UiState }) => state.ui.sidebarCollapsed;

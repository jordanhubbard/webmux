import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { LocalTerminalCloseDialog } from '@frontend/components/LocalTerminalCloseDialog';
import type { Session } from '@frontend/types';
const mock = vi.hoisted(() => ({getSession:vi.fn(),setMinimized:vi.fn(),stopLocalSession:vi.fn(),deleteSession:vi.fn()}));
vi.mock('@frontend/utils/api',()=>({api:mock}));
const session:Session={id:'original',kind:'terminal',owner:'owner',transport:'local',host_id:'',hostname:'localhost',username:'',key_id:'',cols:80,rows:24,row:0,col:0,state:'connected',created_at:'',updated_at:'',title:'Original work',persistent:true,minimized:false,local_lifecycle:{contract:'webmux-local-provider-lifecycle/v1',state:'running',selection:'original-selection'}};
function response(action:{id:string;selection:string},outcome:'stopped'|'uncertain') {return {outcome,session:{...session,local_lifecycle:{...session.local_lifecycle!,state:outcome,selection:action.selection,operation_id:action.id}}};}
beforeEach(()=>{vi.clearAllMocks();mock.getSession.mockResolvedValue(session);mock.setMinimized.mockResolvedValue(session);mock.deleteSession.mockResolvedValue(undefined)});
describe('local terminal close ownership',()=>{
 it('hides with a retained session and never posts Stop or Delete',async()=>{
  const changed=vi.fn(),close=vi.fn();render(<LocalTerminalCloseDialog session={session} onChanged={changed} onClose={close}/>);
  fireEvent.click(screen.getByRole('button',{name:'Hide and keep running'}));await waitFor(()=>expect(close).toHaveBeenCalled());expect(mock.setMinimized).toHaveBeenCalledWith('original',true);expect(mock.stopLocalSession).not.toHaveBeenCalled();expect(mock.deleteSession).not.toHaveBeenCalled();expect(changed).toHaveBeenCalled();
 });
 it('requires deliberate Stop and deletes only after an original-target confirmation',async()=>{
  mock.stopLocalSession.mockImplementation(async(_id,action)=>response(action,'stopped'));
  render(<LocalTerminalCloseDialog session={session} onChanged={vi.fn()} onClose={vi.fn()}/>);
  const button=screen.getByRole('button',{name:'Stop terminal and close'});await waitFor(()=>expect(button).not.toBeDisabled());expect(mock.stopLocalSession).not.toHaveBeenCalled();fireEvent.click(button);
  await waitFor(()=>expect(mock.deleteSession).toHaveBeenCalledWith('original'));expect(mock.stopLocalSession).toHaveBeenCalledWith('original',expect.objectContaining({selection:'original-selection',mode:'stop',confirm:true}));
 });
 it('lost replies retain the original request; inspection never resends Stop',async()=>{
  mock.stopLocalSession.mockRejectedValueOnce(new Error('response lost')).mockImplementation(async(_id,action)=>response(action,'uncertain'));
  render(<LocalTerminalCloseDialog session={session} onChanged={vi.fn()} onClose={vi.fn()}/>);
  const stop=screen.getByRole('button',{name:'Stop terminal and close'});await waitFor(()=>expect(stop).not.toBeDisabled());fireEvent.click(stop);await screen.findByRole('alert');expect(mock.deleteSession).not.toHaveBeenCalled();
  const original=mock.stopLocalSession.mock.calls[0][1];fireEvent.click(screen.getByRole('button',{name:'Inspect original stop'}));await waitFor(()=>expect(mock.stopLocalSession).toHaveBeenCalledTimes(2));
  expect(mock.stopLocalSession.mock.calls[1]).toEqual(['original',{...original,mode:'inspect',confirm:false}]);await screen.findByRole('button',{name:'Continue original stop'});expect(mock.deleteSession).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button',{name:'Continue original stop'}));await waitFor(()=>expect(mock.stopLocalSession).toHaveBeenCalledTimes(3));expect(mock.stopLocalSession.mock.calls[2]).toEqual(['original',{...original,mode:'continue',confirm:true}]);
 });
 it('reconstructs an uncertain request without changing its operation or target',async()=>{
  mock.getSession.mockResolvedValue({...session,local_lifecycle:{...session.local_lifecycle!,state:'requested',operation_id:'saved-operation'}});mock.stopLocalSession.mockImplementation(async(_id,action)=>response(action,'uncertain'));
  render(<LocalTerminalCloseDialog session={session} onChanged={vi.fn()} onClose={vi.fn()}/>);fireEvent.click(await screen.findByRole('button',{name:'Inspect original stop'}));await waitFor(()=>expect(mock.stopLocalSession).toHaveBeenCalledWith('original',{id:'saved-operation',selection:'original-selection',mode:'inspect',confirm:false}));expect(mock.deleteSession).not.toHaveBeenCalled();
 });
 it('refuses a successful reply for a different target',async()=>{
  mock.stopLocalSession.mockImplementation(async(_id,action)=>response({...action,selection:'replacement'},'stopped'));render(<LocalTerminalCloseDialog session={session} onChanged={vi.fn()} onClose={vi.fn()}/>);const button=screen.getByRole('button',{name:'Stop terminal and close'});await waitFor(()=>expect(button).not.toBeDisabled());fireEvent.click(button);await screen.findByRole('alert');expect(mock.deleteSession).not.toHaveBeenCalled();
 });
});

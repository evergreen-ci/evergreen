describe('MDBQueryAdaptorSpec', function() {
  beforeEach(module('MCI'));

  var svc

  beforeEach(inject(function($injector) {
    svc = $injector.get('EvgUtil')
  }))

  it('generates versionId', function() {
    expect(
      svc.generateVersionId({project: 'sys-perf', revision: 'abc123'})
    ).toEqual('sys_perf_abc123')
  })

  it('generates buildId', function() {
    expect(
      svc.generateBuildId({
        project: 'sys-perf',
        revision: 'abc123',
        buildVariant: 'linux-1-node-replSet',
        createTime: '2018-10-06T14:43:50Z'
      })
    ).toEqual('sys_perf_linux_1_node_replSet_abc123_18_10_06_14_43_50')
  })

  it('parses build id', function() {
    expect(
      svc.parseId('0_e2e_local_279a1f51b9a0ba2ccf988a75f99268221e13539d_18_09_14_21_47_13')
    ).toEqual({
      ok: true,
      head: '0_e2e_local',
      revision: '279a1f51b9a0ba2ccf988a75f99268221e13539d',
      createTime: moment.utc([2018, 08, 14, 21, 47, 13])
    })
  })

  it('parses task id', function() {
    expect(
      svc.parseId('cloud_nightly_4.0_e2e_local_E2E_Local_ATM_Deployment_Item_Creation_279a1f51b9a0ba2ccf988a75f99268221e13539d_18_09_14_21_47_13')
    ).toEqual({
      ok: true,
      head: 'cloud_nightly_4.0_e2e_local_E2E_Local_ATM_Deployment_Item_Creation',
      revision: '279a1f51b9a0ba2ccf988a75f99268221e13539d',
      createTime: moment.utc([2018, 08, 14, 21, 47, 13])
    })
  })

  it('parses invalid id', function() {
    expect(
      svc.parseId('qwerty_123')
    ).toEqual({
      ok: false,
      head: '',
      revision: '',
      createTime: moment(0)
    })
  })
})
